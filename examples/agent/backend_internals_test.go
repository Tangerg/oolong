package main

import (
	"context"
	"errors"
	"sync"
	"testing"
	"testing/synctest"

	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/program"
	"github.com/Tangerg/oolong/core/programtest"
)

type bridgeComponent struct{}

func (bridgeComponent) Draw(grid.View)          {}
func (bridgeComponent) Handle(input.Event) bool { return false }

func runningBridge(t *testing.T) *agentBridge {
	t.Helper()
	host := programtest.New(t, programtest.Config{Width: 20, Height: 4})
	ctx, cancel := context.WithCancel(t.Context())
	ready := make(chan *agentBridge, 1)
	done := make(chan error, 1)
	go func() {
		done <- program.Run(ctx, program.Config{
			Host: host,
			Root: func(owner *program.Runtime) program.Component {
				run := &agentRun{}
				ready <- &agentBridge{dispatch: owner.Dispatcher(), owner: &agent{run: run}, run: run}
				return bridgeComponent{}
			},
		})
	}()
	t.Cleanup(func() {
		cancel()
		if err := <-done; err != nil {
			t.Errorf("bridge program: %v", err)
		}
	})
	return <-ready
}

func TestBridgeCancellationWaitsForAClaimedMutation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		bridge := runningBridge(t)
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		entered, release := make(chan struct{}), make(chan struct{})
		unblock := sync.OnceFunc(func() { close(release) })
		defer unblock()
		returned := make(chan error, 1)
		committed := false
		go func() {
			returned <- bridge.post(ctx, func() {
				close(entered)
				<-release
				committed = true
			})
		}()
		<-entered
		cancel()
		synctest.Wait()
		select {
		case err := <-returned:
			t.Fatalf("bridge returned %v while its mutation could still commit", err)
		default:
		}
		unblock()
		if err := <-returned; err != nil {
			t.Fatalf("claimed mutation = %v, want completed mutation", err)
		}
		if !committed {
			t.Fatal("bridge returned before the mutation finished")
		}
	})
}

func TestBridgeCancellationPreventsAPendingMutation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		bridge := runningBridge(t)
		entered, release := make(chan struct{}), make(chan struct{})
		unblock := sync.OnceFunc(func() { close(release) })
		defer unblock()
		bridge.dispatch.Post(func() { close(entered); <-release })
		<-entered
		ctx, cancel := context.WithCancelCause(t.Context())
		defer cancel(nil)
		cause := errors.New("superseded")
		returned := make(chan error, 1)
		committed := false
		go func() { returned <- bridge.post(ctx, func() { committed = true }) }()
		synctest.Wait()
		cancel(cause)
		if err := <-returned; !errors.Is(err, cause) {
			t.Fatalf("pending mutation = %v, want cancellation cause", err)
		}
		unblock()
		if err := bridge.dispatch.Invoke(t.Context(), func() error { return nil }); err != nil {
			t.Fatal(err)
		}
		if committed {
			t.Fatal("cancelled mutation later committed")
		}
	})
}

func TestBridgeKeepsRunIdentityWithTheApplication(t *testing.T) {
	bridge := runningBridge(t)
	if err := bridge.dispatch.Invoke(t.Context(), func() error {
		bridge.owner.run = &agentRun{}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	committed := false
	if err := bridge.post(t.Context(), func() { committed = true }); !errors.Is(err, context.Canceled) {
		t.Fatalf("stale run mutation = %v, want cancellation", err)
	}
	if committed {
		t.Fatal("a superseded run changed its replacement")
	}
}
