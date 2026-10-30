package program

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// deferredCancellation separates an external lifetime ending from scheduling its
// AfterFunc notification. Real schedulers provide the same gap without a bound.
type deferredCancellation struct {
	mu       sync.Mutex
	done     chan struct{}
	err      error
	callback func()
}

func newDeferredCancellation() *deferredCancellation {
	return &deferredCancellation{done: make(chan struct{})}
}

func (*deferredCancellation) Deadline() (time.Time, bool) { return time.Time{}, false }
func (c *deferredCancellation) Done() <-chan struct{}     { return c.done }
func (*deferredCancellation) Value(any) any               { return nil }
func (c *deferredCancellation) Err() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.err
}

func (c *deferredCancellation) AfterFunc(callback func()) func() bool {
	c.mu.Lock()
	c.callback = callback
	c.mu.Unlock()
	return func() bool {
		c.mu.Lock()
		defer c.mu.Unlock()
		if c.callback == nil {
			return false
		}
		c.callback = nil
		return true
	}
}

func (c *deferredCancellation) cancel() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.err == nil {
		c.err = context.Canceled
		close(c.done)
	}
}

func (c *deferredCancellation) flush() {
	c.mu.Lock()
	callback := c.callback
	c.callback = nil
	c.mu.Unlock()
	if callback != nil {
		callback()
	}
}

func TestScopeStartChecksExternalCancellationBeforeItsNotificationRuns(t *testing.T) {
	_, runtime := taskOwner(t)
	external := newDeferredCancellation()
	scope, err := runtime.NewScope(ScopeConfig{Context: external})
	if err != nil {
		t.Fatal(err)
	}
	defer scope.Close()
	defer external.flush()
	external.cancel()
	if scope.Context().Err() != nil {
		t.Fatal("test did not hold the asynchronous cancellation notification")
	}
	_, err = runtime.Start(scope, Work[int]{Run: func(context.Context) (int, error) { return 1, nil }})
	if !errors.Is(err, ErrScopeClosed) {
		t.Fatalf("start crossed an ended external lifetime: %v", err)
	}
}

func TestScopeQueuedCompletionChecksTheExternalLifetimeAtDelivery(t *testing.T) {
	owner, runtime := taskOwner(t)
	external := newDeferredCancellation()
	scope, err := runtime.NewScope(ScopeConfig{Context: external})
	if err != nil {
		t.Fatal(err)
	}
	defer scope.Close()
	defer external.flush()
	applied := false
	task, err := runtime.Start(scope, Work[int]{
		Run:   func(context.Context) (int, error) { return 42, nil },
		Apply: func(int, error) { applied = true },
	})
	if err != nil {
		t.Fatal(err)
	}
	<-task.Done()
	<-owner.tasks.wake
	external.cancel()
	if scope.Context().Err() != nil {
		t.Fatal("test did not hold the asynchronous cancellation notification")
	}
	owner.runTasks()
	<-task.Settled()
	if status := task.Snapshot(); applied || status.State != TaskSucceeded || status.Delivery != DeliveryRevoked || status.Err != nil {
		t.Fatalf("external cancellation changed the outcome or allowed stale delivery: %+v applied=%v", status, applied)
	}
}
