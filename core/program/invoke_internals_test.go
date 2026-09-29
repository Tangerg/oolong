package program

import (
	"context"
	"errors"
	"runtime"
	"testing"
	"testing/synctest"
	"weak"
)

func TestInvokeCancellationStillOwnsWorkTakenButNotYetClaimed(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		queue := newTaskQueue()
		defer queue.stop()
		dispatch := Dispatcher{tasks: queue}
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		called := false
		returned := make(chan error, 1)
		go func() {
			returned <- dispatch.Invoke(ctx, func() error { called = true; return nil })
		}()
		synctest.Wait()
		taken := queue.take()
		if len(taken) != 1 {
			t.Fatalf("taken work = %d, want one invocation", len(taken))
		}
		cancel()
		if err := <-returned; !errors.Is(err, context.Canceled) {
			t.Fatalf("taken Invoke = %v, want cancellation", err)
		}
		taken[0]()
		if called {
			t.Fatal("a task taken before cancellation started after Invoke returned")
		}
	})
}

func TestPendingInvokeReleasesItsCallbackAndContext(t *testing.T) {
	for _, stop := range []bool{false, true} {
		name := "cancelled taken work"
		if stop {
			name = "stopped queued work"
		}
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				queue := newTaskQueue()
				defer queue.stop()
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				returned := make(chan error, 1)
				callbackValue, contextValue := queueInvocationWithValues(ctx, Dispatcher{tasks: queue}, returned)
				synctest.Wait()
				var taken []func()
				want := context.Canceled
				if stop {
					queue.stop()
					want = ErrStopped
				} else {
					taken = queue.take()
					cancel()
				}
				if err := <-returned; !errors.Is(err, want) {
					t.Fatalf("Invoke = %v, want %v", err, want)
				}
				synctest.Wait()
				runtime.GC()
				if callbackValue.Value() != nil || contextValue.Value() != nil {
					t.Fatal("unapplied invocation retained its callback or context payload")
				}
				runtime.KeepAlive(taken)
				runtime.KeepAlive(queue)
			})
		})
	}
}

type invocationContextKey struct{}

func queueInvocationWithValues(ctx context.Context, dispatch Dispatcher, returned chan<- error) (weak.Pointer[[64]byte], weak.Pointer[[64]byte]) {
	callbackValue, contextValue := new([64]byte), new([64]byte)
	callbackRef, contextRef := weak.Make(callbackValue), weak.Make(contextValue)
	ctx = context.WithValue(ctx, invocationContextKey{}, contextValue)
	go func() {
		returned <- dispatch.Invoke(ctx, func() error {
			runtime.KeepAlive(callbackValue)
			return nil
		})
	}()
	return callbackRef, contextRef
}
