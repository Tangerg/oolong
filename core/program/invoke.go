package program

import (
	"context"
	"errors"
	"sync"
)

// ErrInvocationAborted means an invoked callback exited without returning. A panic
// still propagates from Run; the waiting caller receives this error instead of
// waiting forever or mistaking the callback for a successful operation.
var ErrInvocationAborted = errors.New("program: invoked callback did not return")

// Invoke runs fn on the interface goroutine in Post acceptance order and returns
// its error. It must be called from another goroutine: the interface owner cannot
// wait for work that only it can run. A nil fn returns an error; a nil ctx panics.
//
// Cancellation before the callback is claimed prevents it from starting and returns
// context.Cause(ctx). Once claimed, Invoke waits for the callback to finish even if
// ctx is cancelled, so the callback can never start or continue after Invoke returns.
// A zero or stopped dispatcher returns ErrStopped for unapplied work. If stopping
// and cancellation compete, either cause may win before the callback is claimed.
//
// A callback panic or goroutine exit returns ErrInvocationAborted to the caller;
// panics still unwind Run with their original value. Normal completion requests a
// frame, but Invoke does not wait for that frame to be drawn or written.
func (d Dispatcher) Invoke(ctx context.Context, fn func() error) error {
	if fn == nil {
		return errors.New("program: invoke requires a callback")
	}
	if err := ctx.Err(); err != nil {
		return context.Cause(ctx)
	}
	call := &invocation{
		pending: func() error {
			if err := ctx.Err(); err != nil {
				return context.Cause(ctx)
			}
			return fn()
		},
		done: make(chan struct{}),
	}
	if !d.post(call.run) {
		return ErrStopped
	}
	select {
	case <-call.done:
	case <-ctx.Done():
		call.cancel(context.Cause(ctx))
	case <-d.Done():
		call.cancel(ErrStopped)
	}
	<-call.done
	return call.err
}

// Taking pending under mu gives run or cancel sole ownership of completion.
// Clearing it also releases the callback and its context while a cancelled task
// is still waiting in the dispatcher's FIFO.
type invocation struct {
	mu      sync.Mutex
	pending func() error
	done    chan struct{}
	err     error
}

func (i *invocation) run() {
	i.mu.Lock()
	fn := i.pending
	i.pending = nil
	i.mu.Unlock()
	if fn == nil {
		return
	}
	err := ErrInvocationAborted
	defer func() { i.complete(err) }()
	err = fn()
}

func (i *invocation) cancel(err error) {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.pending == nil {
		return
	}
	i.pending = nil
	i.complete(err)
}

func (i *invocation) complete(err error) {
	i.err = err
	close(i.done)
}
