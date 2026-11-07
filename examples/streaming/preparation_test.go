package main

import (
	"context"
	"io"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"

	"github.com/Tangerg/oolong/ui/core/content"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/program"
)

type preparedText string

func (preparedText) HeightForWidth(int) int { return 1 }
func (p preparedText) Draw(view grid.View)  { view.Text(0, 0, string(p), grid.Style{}) }

func TestPreparationWaitsForExecutionAndKeepsTheLatestEOF(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHost(t)
		c, stop := runSource(t, h, func(context.Context, string, io.Writer) error { return nil })
		defer stop()
		release := make(chan struct{})
		var released bool
		defer func() {
			if !released {
				close(release)
			}
		}()
		entered := make(chan string, 8)
		var active, peak atomic.Int32
		if err := c.runtime.Dispatcher().Invoke(t.Context(), func() error {
			var err error
			c.document, err = content.NewStreamDocument(content.StreamConfig{Format: "markdown"})
			if err != nil {
				return err
			}
			c.item, err = c.content.AddDocument(c.document)
			c.render = func(_ context.Context, source string) (grid.Drawable, error) {
				n := active.Add(1)
				defer active.Add(-1)
				for previous := peak.Load(); n > previous; previous = peak.Load() {
					if peak.CompareAndSwap(previous, n) {
						break
					}
				}
				entered <- source
				// Goldmark's entered parse cannot be interrupted by cancelling ctx.
				<-release
				return preparedText(source), nil
			}
			c.accept(c.document, program.ByteBatch{Data: []byte("first ")})
			return err
		}); err != nil {
			t.Fatal(err)
		}
		synctest.Wait()
		if len(entered) != 1 {
			t.Fatalf("initial executions = %d", len(entered))
		}
		if err := c.runtime.Dispatcher().Invoke(t.Context(), func() error {
			for range 3 {
				c.accept(c.document, program.ByteBatch{Data: []byte("later ")})
			}
			c.accept(c.document, program.ByteBatch{Final: true})
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		synctest.Wait()
		if peak.Load() != 1 || len(entered) != 1 {
			t.Fatalf("a pending revision overlapped the parser: peak=%d executions=%d", peak.Load(), len(entered))
		}
		close(release)
		released = true
		synctest.Wait()
		if peak.Load() != 1 || len(entered) != 2 {
			t.Fatalf("intermediate revisions were not coalesced: peak=%d executions=%d", peak.Load(), len(entered))
		}
		<-entered
		if latest := <-entered; latest != "first "+strings.Repeat("later ", 3) {
			t.Fatalf("latest final source = %q", latest)
		}
		if got := stateOf(t, c); got.active || got.open || got.status != "complete" {
			t.Fatalf("EOF did not settle: %+v", got)
		}
	})
}
