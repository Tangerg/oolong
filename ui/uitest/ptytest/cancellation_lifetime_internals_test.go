package ptytest

import (
	"context"
	"errors"
	"runtime"
	"testing"
	"weak"
)

func TestStartRefusesCancelledContextWithItsCause(t *testing.T) {
	ctx, cancel := context.WithCancelCause(t.Context())
	cause := errors.New("launch withdrawn")
	cancel(cause)
	session, err := Start(ctx, Config{}, "sh", "-c", "exit 0")
	if session != nil {
		t.Cleanup(func() { session.close() })
		t.Error("cancelled context launched a session")
	}
	if !errors.Is(err, cause) {
		t.Fatalf("cancelled Start = %v, want its cause", err)
	}
}

func TestClosedSessionReleasesItsContextValues(t *testing.T) {
	if !Supported() {
		t.Skip("no PTY on this platform")
	}
	session, supplied, err := func() (*Session, weak.Pointer[[64]byte], error) {
		type contextKey struct{}
		value := new([64]byte)
		ctx := context.WithValue(t.Context(), contextKey{}, value)
		reference := weak.Make(value)
		session, err := Start(ctx, Config{}, "sh", "-c", "exit 0")
		return session, reference, err
	}()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { session.close() })
	if err := session.Wait(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	runtime.GC()
	retained := supplied.Value() != nil
	runtime.KeepAlive(session)
	if retained {
		t.Fatal("closed session retained values belonging to its caller's context")
	}
}

type interruptedCancellationContext struct {
	context.Context //nolint:containedctx // This wrapper exercises a caller-owned cancellation hook.
	stop            func()
}

// The parent's private cancellation key would bypass this context's observation hook.
func (*interruptedCancellationContext) Value(any) any { return nil }

func (c *interruptedCancellationContext) AfterFunc(callback func()) func() bool {
	stop := context.AfterFunc(c.Context, callback)
	return func() bool {
		stopped := stop()
		c.stop()
		return stopped
	}
}

func TestExternalCancellationCleanupCannotStrandSessionResources(t *testing.T) {
	if !Supported() {
		t.Skip("no PTY on this platform")
	}
	for _, exit := range []string{"panic", "Goexit"} {
		t.Run(exit, func(t *testing.T) {
			failure := "external cancellation cleanup interrupted"
			ctx := &interruptedCancellationContext{Context: t.Context(), stop: func() {
				if exit == "Goexit" {
					runtime.Goexit()
				}
				panic(failure)
			}}
			session, err := Start(ctx, Config{}, "sh", "-c", "exit 0")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { session.close() })
			if err := session.Wait(t.Context()); err != nil {
				t.Fatal(err)
			}
			done := make(chan struct{})
			go func() {
				defer close(done)
				defer func() {
					if recovered := recover(); recovered != nil && recovered != failure {
						t.Errorf("Close panic = %v", recovered)
					}
				}()
				if err := session.Close(); err != nil {
					t.Error(err)
				}
			}()
			<-done
			for _, resource := range []struct {
				name string
				done <-chan struct{}
			}{
				{"process identity", session.release},
				{"reader", session.transcript.done},
				{"reaper", session.reapDone},
			} {
				select {
				case <-resource.done:
				default:
					t.Errorf("Close left %s owned after external cancellation cleanup", resource.name)
				}
			}
		})
	}
}
