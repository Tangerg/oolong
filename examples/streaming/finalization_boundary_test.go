package main

import (
	"context"
	"io"
	"sync"
	"testing"
	"testing/synctest"

	"github.com/Tangerg/oolong/ui/core/content"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/program"
)

func TestCancelSourceDoesNotMisreportFinalPreparation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		host := newHost(t)
		sourceDone := make(chan struct{})
		c, stop := runSource(t, host, func(ctx context.Context, _ string, _ io.Writer) error {
			select {
			case <-sourceDone:
				return nil
			case <-ctx.Done():
				return context.Cause(ctx)
			}
		})
		defer stop()
		entered, gate := make(chan struct{}), make(chan struct{})
		release := sync.OnceFunc(func() { close(gate) })
		defer release()
		var document *content.StreamDocument
		if err := c.runtime.Dispatcher().Invoke(t.Context(), func() error {
			c.render = func(_ context.Context, source string) (grid.Drawable, error) {
				close(entered)
				<-gate
				return preparedText(source), nil
			}
			c.startReply("finish the source")
			document = c.document
			// Keep the renderer cold until the real source EOF admits its final revision.
			return document.Append([]byte("final answer"))
		}); err != nil {
			t.Fatal(err)
		}
		close(sourceDone)
		<-entered
		synctest.Wait()
		if err := c.runtime.Dispatcher().Invoke(t.Context(), func() error {
			if c.document != document || document.Current().State != content.Finalizing || c.producer.Snapshot().State != program.TaskSucceeded || c.status.Doing != "preparing final answer" {
				t.Errorf("source EOF did not enter final preparation: document=%+v producer=%+v status=%q", document.Current(), c.producer.Snapshot(), c.status.Doing)
			}
			before := c.status.Doing
			c.stopSource()
			if c.status.Doing != before || c.document != document || document.Current().State != content.Finalizing {
				t.Errorf("cancel-source changed finished source: document=%+v status=%q", document.Current(), c.status.Doing)
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		release()
		synctest.Wait()
		if err := c.runtime.Dispatcher().Invoke(t.Context(), func() error {
			if c.document != nil || c.producer != nil || document.Current().State != content.Final || c.status.Doing != "complete" {
				t.Errorf("prepared final answer did not complete: document=%+v producer=%+v status=%q", document.Current(), c.producer.Snapshot(), c.status.Doing)
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	})
}
