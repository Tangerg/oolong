package main

import (
	"context"
	"errors"
	"sync"
	"testing"
	"testing/synctest"

	"github.com/Tangerg/oolong/examples/internal/docprep"
	"github.com/Tangerg/oolong/extensions/markdown"
	"github.com/Tangerg/oolong/ui/core/content"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/program"
)

func TestFinalPreparationRefusalSettlesItsDocument(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		fixture := newTestConversation(t, 8, nil)
		entered, gate := make(chan struct{}), make(chan struct{})
		release := sync.OnceFunc(func() { close(gate) })
		defer release()
		if err := fixture.runtime.Dispatcher().Invoke(t.Context(), func() error {
			document, err := content.NewStreamDocument(content.StreamConfig{Format: markdown.Format})
			if err != nil {
				return err
			}
			return fixture.conversation.prepare.Prepare(document, func(context.Context, string) (grid.Drawable, error) {
				close(entered)
				<-gate
				return preparedAnswer{}, nil
			}, nil)
		}); err != nil {
			t.Fatal(err)
		}
		<-entered
		var refused *content.StreamDocument
		sourceErr := errors.New("source disconnected")
		if err := fixture.runtime.Dispatcher().Invoke(t.Context(), func() error {
			c := fixture.conversation
			full := false
			for range 256 {
				document, err := content.NewStreamDocument(content.StreamConfig{Format: markdown.Format})
				if err != nil {
					return err
				}
				err = c.prepare.Prepare(document, func(context.Context, string) (grid.Drawable, error) { return preparedAnswer{}, nil }, nil)
				if errors.Is(err, docprep.ErrCapacity) {
					full = true
					break
				}
				if err != nil {
					return err
				}
			}
			if !full {
				return errors.New("pending preparation did not reach its admission bound")
			}
			if _, err := c.beginMarkdown(); err != nil {
				return err
			}
			refused = c.document
			if err := refused.Append([]byte("rejected final answer")); err != nil {
				return err
			}
			if err := c.FinishMarkdown(t.Context(), sourceErr); !errors.Is(err, docprep.ErrCapacity) {
				t.Errorf("final admission = %v, want capacity refusal", err)
			}
			change := refused.Current()
			if c.document != nil || !refused.Snapshot().Final() || change.State != content.Failed || change.PreparedRevision != change.Revision || !errors.Is(change.SourceError, sourceErr) || !errors.Is(change.PrepareError, docprep.ErrCapacity) {
				t.Errorf("refused EOF has no settled owner: %+v; receiving=%t", change, c.document != nil)
			}
			c.prepare.Clear()
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		release()
		synctest.Wait()
		if err := fixture.runtime.Dispatcher().Invoke(t.Context(), func() error {
			if change := refused.Current(); change.State != content.Failed || !errors.Is(change.SourceError, sourceErr) || !errors.Is(change.PrepareError, docprep.ErrCapacity) {
				t.Errorf("later worker completion changed refused EOF: %+v", change)
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	})
}

type finalizingBackend struct {
	started chan<- *agentBridge
	finish  <-chan struct{}
}

func (b finalizingBackend) Run(ctx context.Context, _ runRequest, output agentOutput) error {
	b.started <- output.(*agentBridge)
	select {
	case <-b.finish:
		return nil
	case <-ctx.Done():
		return context.Cause(ctx)
	}
}

func TestCancelSourceDoesNotMisreportFinalPreparation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		started, sourceDone := make(chan *agentBridge, 1), make(chan struct{})
		host, stop := runAgent(t, finalizingBackend{started: started, finish: sourceDone})
		defer stop()
		entered, gate := make(chan struct{}), make(chan struct{})
		release := sync.OnceFunc(func() { close(gate) })
		defer release()
		host.Type("finish the source")
		host.Press(input.Enter)
		bridge := <-started
		if err := bridge.dispatch.Invoke(t.Context(), func() error {
			c := bridge.owner.conversation
			c.render = func(context.Context, string) (grid.Drawable, error) {
				close(entered)
				<-gate
				return preparedAnswer{}, nil
			}
			// Keep the renderer cold until the real source EOF admits its final revision.
			if _, err := c.beginMarkdown(); err != nil {
				return err
			}
			return c.document.Append([]byte("final answer"))
		}); err != nil {
			t.Fatal(err)
		}
		close(sourceDone)
		<-entered
		synctest.Wait()
		if err := bridge.dispatch.Invoke(t.Context(), func() error {
			a := bridge.owner
			if a.run != bridge.run || a.run.phase != runFinishing || a.run.task.Snapshot().State != program.TaskSucceeded || a.run.status.Doing != "preparing final answer" {
				t.Errorf("source EOF did not enter final preparation: run=%+v", a.run)
			}
			before := a.run.status.Doing
			a.stopSource()
			if a.run != bridge.run || a.run.status.Doing != before || a.outcome != (sessionOutcome{}) {
				t.Errorf("cancel-source changed finished source: run=%+v outcome=%+v", a.run, a.outcome)
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		release()
		synctest.Wait()
		if err := bridge.dispatch.Invoke(t.Context(), func() error {
			a := bridge.owner
			if a.run != nil || a.outcome.kind != outcomeCompleted {
				t.Errorf("prepared final answer did not complete: run=%+v outcome=%+v", a.run, a.outcome)
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	})
}
