package main

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/grid"
)

func TestAnEmptyOpenProjectionReplacesTheVisibleTail(t *testing.T) {
	c := &chat{}
	c.view = kit.Transcript{Content: &c.content, Scroll: &c.scroll}
	c.acceptMarkdown("<")
	if got := drawTail(c); !strings.Contains(got, "<") {
		t.Fatalf("the incomplete source was not visible: %q", got)
	}
	c.acceptMarkdown("!-- hidden -->")
	if got := drawTail(c); strings.Contains(got, "<") {
		t.Fatalf("the empty projection retained the old tail: %q", got)
	}
}

func drawTail(c *chat) string {
	surface := grid.NewSurface(40, 8)
	headless.NewRoot(&c.view).Draw(surface.View())
	var drawn strings.Builder
	for y := range 8 {
		for x := range 40 {
			cell, _ := surface.CellAt(x, y)
			drawn.WriteString(cell.Content())
		}
	}
	return drawn.String()
}

func TestAnEmptyFinalProjectionStillCompletesTheTranscript(t *testing.T) {
	for _, ending := range []struct {
		name string
		err  error
	}{
		{name: "complete"},
		{name: "failed", err: errors.New("source failed")},
		{name: "cancelled", err: context.Canceled},
	} {
		t.Run(ending.name, func(t *testing.T) {
			h := newHost()
			t.Cleanup(func() {
				if err := h.writer.Close(); err != nil {
					t.Errorf("closing the test writer: %v", err)
				}
			})
			chunks := make(chan string)
			c, stop := runSource(t, h, func(ctx context.Context, _ string, dst io.Writer) error {
				for {
					select {
					case <-ctx.Done():
						return context.Cause(ctx)
					case chunk, ok := <-chunks:
						if !ok {
							return ending.err
						}
						if _, err := io.WriteString(dst, chunk); err != nil {
							return err
						}
					}
				}
			})
			defer stop()
			approve(t, h, c, "show a comment")
			chunks <- "<"
			awaitState(t, c, func(s state) bool { return s.open })
			chunks <- "!-- hidden -->"
			close(chunks)
			awaitState(t, c, func(s state) bool { return !s.active })
			if err := c.runtime.Dispatcher().Invoke(t.Context(), func() error {
				for i := range c.content.Len() {
					id := c.content.FirstBlock() + headless.BlockID(i)
					if !c.content.Finished(id) {
						return errors.New("source completion left an unfinished transcript block")
					}
				}
				for range retainedFinished + 3 {
					c.appendFinished(&kit.Label{Text: "later output"})
				}
				c.retainWindow()
				if c.content.Len() > retainedFinished {
					return errors.New("the settled tail blocked later scrollback publication")
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
		})
	}
}
