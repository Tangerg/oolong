package program_test

import (
	"context"
	"errors"
	"runtime"
	"slices"
	"strings"
	"testing"
	"testing/synctest"

	"github.com/Tangerg/oolong/core/program"
)

func startWithDispatcher(t *testing.T) (*running, program.Dispatcher) {
	t.Helper()
	ready := make(chan program.Dispatcher, 1)
	r := start(t, func(root *component) { ready <- root.runtime.Dispatcher() })
	return r, <-ready
}

func TestInvokePreservesPostOrderAndReturnsTheCallbackError(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		r, dispatch := startWithDispatcher(t)
		cause := errors.New("transition refused")
		var order []string
		dispatch.Post(func() { order = append(order, "before") })
		if err := dispatch.Invoke(t.Context(), func() error {
			order = append(order, "invoke")
			return cause
		}); !errors.Is(err, cause) {
			t.Fatalf("Invoke = %v, want callback error", err)
		}
		dispatch.Post(func() { order = append(order, "after") })
		if err := dispatch.Invoke(t.Context(), func() error {
			r.root.text = "invoked frame"
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(order, []string{"before", "invoke", "after"}) {
			t.Fatalf("callback order = %v", order)
		}
		r.until("invoked frame", func() bool { return strings.Contains(r.host.frames.String(), "invoked frame") })
		dispatch.Post(r.root.runtime.Quit)
		if err := r.wait(); err != nil {
			t.Fatal(err)
		}
	})
}

func TestInvokeRejectsZeroAndStoppedDispatchers(t *testing.T) {
	var zero program.Dispatcher
	called := false
	callback := func() error { called = true; return nil }
	if err := zero.Invoke(t.Context(), callback); !errors.Is(err, program.ErrStopped) {
		t.Fatalf("zero Invoke = %v, want ErrStopped", err)
	}
	r, dispatch := startWithDispatcher(t)
	dispatch.Post(r.root.runtime.Quit)
	if err := r.wait(); err != nil {
		t.Fatal(err)
	}
	if err := dispatch.Invoke(t.Context(), callback); !errors.Is(err, program.ErrStopped) {
		t.Fatalf("stopped Invoke = %v, want ErrStopped", err)
	}
	if called {
		t.Fatal("dispatcher without an owner invoked a callback")
	}
}

func TestInvokeRejectsMissingCallbacksAndAlreadyCancelledContexts(t *testing.T) {
	r, dispatch := startWithDispatcher(t)
	if err := dispatch.Invoke(t.Context(), nil); err == nil {
		t.Fatal("nil callback was accepted")
	}
	ctx, cancel := context.WithCancelCause(t.Context())
	cause := errors.New("operation superseded")
	cancel(cause)
	called := false
	if err := dispatch.Invoke(ctx, func() error { called = true; return nil }); !errors.Is(err, cause) {
		t.Fatalf("cancelled Invoke = %v, want cancellation cause", err)
	}
	dispatch.Post(r.root.runtime.Quit)
	if err := r.wait(); err != nil {
		t.Fatal(err)
	}
	if called {
		t.Fatal("an already cancelled callback ran")
	}
}

func TestInvokeCancellationPreventsPendingWorkFromStarting(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		r, dispatch := startWithDispatcher(t)
		entered, release := make(chan struct{}), make(chan struct{})
		dispatch.Post(func() { close(entered); <-release })
		<-entered
		ctx, cancel := context.WithCancelCause(t.Context())
		cause := errors.New("operation superseded")
		returned := make(chan error, 1)
		called := false
		go func() {
			returned <- dispatch.Invoke(ctx, func() error { called = true; return nil })
		}()
		synctest.Wait()
		cancel(cause)
		if err := <-returned; !errors.Is(err, cause) {
			t.Fatalf("pending Invoke = %v, want cancellation cause", err)
		}
		close(release)
		dispatch.Post(r.root.runtime.Quit)
		if err := r.wait(); err != nil {
			t.Fatal(err)
		}
		if called {
			t.Fatal("callback started after cancellation returned")
		}
	})
}

func TestInvokeCancellationWaitsForClaimedWorkAndPreservesItsResult(t *testing.T) {
	for _, result := range []error{nil, errors.New("commit failed")} {
		name := "success"
		if result != nil {
			name = "error"
		}
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				r, dispatch := startWithDispatcher(t)
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				entered, release := make(chan struct{}), make(chan struct{})
				returned := make(chan error, 1)
				committed := false
				go func() {
					returned <- dispatch.Invoke(ctx, func() error {
						close(entered)
						<-release
						committed = true
						return result
					})
				}()
				<-entered
				cancel()
				synctest.Wait()
				select {
				case err := <-returned:
					t.Fatalf("claimed callback was still running when Invoke returned %v", err)
				default:
				}
				close(release)
				if err := <-returned; !errors.Is(err, result) {
					t.Fatalf("claimed Invoke = %v, want callback result %v", err, result)
				}
				if !committed {
					t.Fatal("Invoke returned before the callback committed")
				}
				dispatch.Post(r.root.runtime.Quit)
				if err := r.wait(); err != nil {
					t.Fatal(err)
				}
			})
		})
	}
}

func TestInvokeReturnsStoppedWhenQuitDiscardsItsTakenBatch(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ready := make(chan program.Dispatcher, 1)
		release := make(chan struct{})
		r := start(t, func(root *component) {
			dispatch := root.runtime.Dispatcher()
			dispatch.Post(root.runtime.Quit)
			ready <- dispatch
			<-release
		})
		dispatch := <-ready
		returned := make(chan error, 1)
		called := false
		go func() {
			returned <- dispatch.Invoke(t.Context(), func() error { called = true; return nil })
		}()
		synctest.Wait()
		close(release)
		if err := <-returned; !errors.Is(err, program.ErrStopped) {
			t.Fatalf("discarded Invoke = %v, want ErrStopped", err)
		}
		if err := r.wait(); err != nil {
			t.Fatal(err)
		}
		if called {
			t.Fatal("callback ran after Quit ended its batch")
		}
	})
}

func TestInvokePanicReleasesTheCallerAndStillUnwindsRun(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		host := newHost(t)
		ready := make(chan program.Dispatcher, 1)
		panicked := make(chan any, 1)
		panicValue := &struct{ message string }{message: "callback panic"}
		go func() {
			defer func() { panicked <- recover() }()
			_ = program.Run(t.Context(), program.Config{
				Host: host,
				Root: func(owner *program.Runtime) program.Component {
					ready <- owner.Dispatcher()
					return &component{text: "ready"}
				},
			})
		}()
		dispatch := <-ready
		if err := dispatch.Invoke(t.Context(), func() error { panic(panicValue) }); !errors.Is(err, program.ErrInvocationAborted) {
			t.Fatalf("panicking Invoke = %v, want ErrInvocationAborted", err)
		}
		if got := <-panicked; got != panicValue {
			t.Fatalf("Run panic = %v, want original panic %v", got, panicValue)
		}
		select {
		case <-dispatch.Done():
		default:
			t.Fatal("panic did not stop the dispatcher")
		}
	})
}

func TestInvokeGoroutineExitReleasesTheCaller(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		host := newHost(t)
		ready := make(chan program.Dispatcher, 1)
		stopped := make(chan struct{})
		go func() {
			defer close(stopped)
			_ = program.Run(t.Context(), program.Config{
				Host: host,
				Root: func(owner *program.Runtime) program.Component {
					ready <- owner.Dispatcher()
					return &component{text: "ready"}
				},
			})
		}()
		dispatch := <-ready
		if err := dispatch.Invoke(t.Context(), func() error { runtime.Goexit(); return nil }); !errors.Is(err, program.ErrInvocationAborted) {
			t.Fatalf("exited Invoke = %v, want ErrInvocationAborted", err)
		}
		<-stopped
	})
}

func TestInvokeAcceptanceRacingWithStopAlwaysSettles(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		for range 100 {
			r, dispatch := startWithDispatcher(t)
			gate := make(chan struct{})
			returned := make(chan error, 1)
			called := false
			go func() {
				<-gate
				returned <- dispatch.Invoke(t.Context(), func() error { called = true; return nil })
			}()
			go func() {
				<-gate
				dispatch.Post(r.root.runtime.Quit)
			}()
			close(gate)
			err := <-returned
			if err != nil && !errors.Is(err, program.ErrStopped) {
				t.Fatalf("racing Invoke = %v", err)
			}
			if waitErr := r.wait(); waitErr != nil {
				t.Fatal(waitErr)
			}
			if called != (err == nil) {
				t.Fatalf("callback ran = %v, Invoke = %v", called, err)
			}
		}
	})
}
