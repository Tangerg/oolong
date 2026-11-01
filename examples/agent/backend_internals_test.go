package main

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/Tangerg/oolong/ui/core/input"

	"github.com/Tangerg/oolong/ui/core/program"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/uitest"
)

func runningBridge(t *testing.T) *agentBridge {
	t.Helper()
	host := uitest.New(t, uitest.Config{Width: 20, Height: 4})
	ctx, cancel := context.WithCancel(t.Context())
	ready := make(chan *agentBridge, 1)
	done := make(chan error, 1)
	go func() {
		app, err := program.NewScreen(program.ScreenConfig{Host: host, Build: func(owner *program.Runtime) (scene.Element, error) {
			run := &agentRun{}
			ready <- &agentBridge{dispatch: owner.Dispatcher(), owner: &agent{run: run}, run: run}
			return scene.NewElement(scene.ElementConfig{}), nil
		}})
		if err == nil {
			err = app.Run(ctx)
		}
		done <- err
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
		if err := bridge.dispatch.Post(func() { close(entered); <-release }); err != nil {
			t.Fatal(err)
		}
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

type captureRuns struct{ started chan *agentBridge }

func (b captureRuns) Run(ctx context.Context, _ string, output agentOutput) error {
	b.started <- output.(*agentBridge)
	<-ctx.Done()
	return context.Cause(ctx)
}

func TestLateIngressCannotFinishTheNextAgentRun(t *testing.T) {
	started := make(chan *agentBridge, 2)
	host, stop := runAgent(t, captureRuns{started})
	defer stop()
	host.Shows(t, "Ask the mock agent")
	host.Type("first")
	host.Press(input.Enter)
	first := <-started
	if err := first.dispatch.Invoke(t.Context(), func() error {
		if _, err := first.Write([]byte("late first run text")); err != nil {
			return err
		}
		first.owner.finishRun(errors.New("local result failed"))
		first.owner.startRun("second")
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	second := <-started
	<-first.ingress.Done()
	if err := second.dispatch.Inspect(t.Context(), func() error {
		if second.owner.run != second.run || second.owner.conversation.document != nil || !strings.Contains(second.owner.status.Doing, "planning") {
			t.Errorf("old bytes or EOF changed the next run: %s", second.owner.status.Doing)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestSemanticDismissAnswersTheWaitingReview(t *testing.T) {
	started := make(chan *agentBridge, 1)
	host, stop := runAgent(t, captureRuns{started})
	defer stop()
	host.Type("review")
	host.Press(input.Enter)
	bridge := <-started
	type outcome struct {
		approved bool
		err      error
	}
	replied := make(chan outcome, 1)
	go func() {
		yes, err := bridge.Review(t.Context(), changeProposal{Path: "main.go", Summary: "review change"})
		replied <- outcome{yes, err}
	}()
	host.Shows(t, "Review tool call")
	if err := bridge.dispatch.Invoke(t.Context(), func() error {
		a := bridge.owner
		_, err := a.runtime.Scene().Invoke(t.Context(), a.reviewDialog.Element().Target(), scene.ActionDismiss, nil)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-replied:
		if got.approved || got.err != nil {
			t.Fatalf("dismissal returned %+v", got)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("dismissed a dialog without answering its backend")
	}
}
