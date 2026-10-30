package main

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/Tangerg/oolong/extensions/markdown"
	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/content"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/kit"
)

func TestAnEmptyOpenProjectionReplacesTheVisibleTail(t *testing.T) {
	document, err := content.NewStreamDocument(content.StreamConfig{Format: "markdown"})
	if err != nil {
		t.Fatal(err)
	}
	view, err := kit.NewTranscript(kit.TranscriptConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if _, addErr := view.Controller().AddDocument(document); addErr != nil {
		t.Fatal(addErr)
	}
	root, err := scene.NewRoot(scene.RootConfig{Element: view.Element()})
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	render := markdown.NewRenderer(markdown.RendererConfig{})
	for i, chunk := range []string{"<", "!-- hidden -->"} {
		if err := document.Append([]byte(chunk)); err != nil {
			t.Fatal(err)
		}
		if err := document.Accept(document.Snapshot().Prepare(t.Context(), render)); err != nil {
			t.Fatal(err)
		}
		surface := grid.NewSurface(40, 8)
		frame, err := root.Prepare(surface.View())
		if err != nil {
			t.Fatal(err)
		}
		frame.Commit(uint64(i + 1))
		root.Wrote(uint64(i + 1))
		var shown strings.Builder
		for y := range 8 {
			for x := range 40 {
				cell, _ := surface.CellAt(x, y)
				shown.WriteString(cell.Content())
			}
		}
		if strings.Contains(shown.String(), "<") != (i == 0) {
			t.Fatalf("wrong provisional projection: %q", shown.String())
		}
	}
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
				if c.item.State() == base.ContentLive {
					return errors.New("source completion left a live transcript item")
				}
				for range 7 {
					c.appendFinished(&kit.Label{Text: "later output"})
				}
				if c.content.Len() != 0 {
					return errors.New("the settled tail blocked later publication")
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
		})
	}
}
