package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/program"
	"github.com/Tangerg/oolong/ui/core/publication"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/kit"
)

// draw gives the conversation a width, which is what everything it owns is measured
// against, and a height, which is what decides whether it is scrolled.
func draw(t *testing.T, c *conversation, height int) {
	t.Helper()
	drawn(t, c, height)
}

func drawn(t *testing.T, c *conversation, height int) string {
	t.Helper()
	const width = 40
	surface := grid.NewSurface(width, height)
	root, err := scene.NewRoot(scene.RootConfig{Element: c.Element()})
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	frame, err := root.Prepare(surface.View())
	if err != nil {
		t.Fatal(err)
	}
	frame.Commit(1)
	root.Wrote(1)
	var out strings.Builder
	for y := range height {
		for x := range width {
			cell, ok := surface.CellAt(x, y)
			if !ok || cell.Width() == 0 {
				continue
			}
			out.WriteString(cell.Content())
		}
	}
	return out.String()
}

func newTestConversation(t *testing.T) *conversation {
	t.Helper()
	return newConversation(kit.Theme{}, kit.GlyphsFor(""), input.Wheel{}, nil)
}

func TestAnAnswerThatEndsWithNothingLeftStillEnds(t *testing.T) {
	// Retention stops at the first block that has not finished, so a block left open
	// for good takes everything after it with it: the terminal's own scrollback never
	// receives another line of the session.
	publisher, err := publication.New(publication.Config{Width: 40})
	if err != nil {
		t.Fatal(err)
	}
	c := newConversation(kit.Theme{}, kit.GlyphsFor(""), input.Wheel{}, publisher)
	draw(t, c, 10)
	first, err := c.beginMarkdown()
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Markdown(t.Context(), "<"); err != nil {
		t.Fatal(err)
	}
	if err := c.Markdown(t.Context(), "!-- hidden -->"); err != nil {
		t.Fatal(err)
	}
	if err := c.FinishMarkdown(t.Context(), nil); err != nil {
		t.Fatal(err)
	}
	if first.State() == base.ContentLive {
		t.Fatal("the answer remained live after EOF")
	}
	for range 20 {
		if err := c.Markdown(t.Context(), "a line\n\n"); err != nil {
			t.Fatal(err)
		}
		if err := c.FinishMarkdown(t.Context(), nil); err != nil {
			t.Fatal(err)
		}
	}
	if c.content.Len() != 8 || first.State() != base.ContentRetired || first.Receipt().Snapshot().ID == 0 {
		t.Fatal("settled content did not transfer to the publisher")
	}
}

func TestMoreOfTheSameAnswerDoesNotTakeTheReaderBackToIt(t *testing.T) {
	// A reader who scrolled up is reading. An answer still being written arrives in a
	// great many pieces, and the piece that happens to arrive with no open block
	// behind it is no more a reason to move them than any of the others.
	c := newTestConversation(t)
	if err := c.User(t.Context(), strings.Repeat("a long prompt that fills the window\n", 20)); err != nil {
		t.Fatal(err)
	}
	if err := c.Markdown(t.Context(), "first\n\n<!-- separator -->\n\n"); err != nil {
		t.Fatal(err)
	}
	draw(t, c, 6)
	c.content.Scroll().ToTop()
	if c.content.Scroll().FollowingEnd() {
		t.Fatal("the reader is at the end after scrolling to the top")
	}

	if err := c.Markdown(t.Context(), "second\n\nthird\n"); err != nil {
		t.Fatal(err)
	}
	draw(t, c, 6)
	if c.content.Scroll().FollowingEnd() {
		t.Fatal("more of the same answer took the reader back to the end of it")
	}
}

func TestTextThatTurnedOutToBeACommentStopsBeingOnTheScreen(t *testing.T) {
	// What is open is what the source says so far, and once the comment is closed
	// the source says nothing. Leaving the last rendering that was not empty means a
	// reader goes on seeing a stray "<" the document does not contain.
	for _, settle := range []string{"open", "flushed"} {
		t.Run(settle, func(t *testing.T) {
			c := newTestConversation(t)
			if err := c.Markdown(t.Context(), "<"); err != nil {
				t.Fatal(err)
			}
			draw(t, c, 8)
			if err := c.Markdown(t.Context(), "!-- hidden -->"); err != nil {
				t.Fatal(err)
			}
			if settle == "flushed" {
				if err := c.FinishMarkdown(t.Context(), nil); err != nil {
					t.Fatal(err)
				}
			}
			if got := drawn(t, c, 8); strings.Contains(got, "<") {
				t.Fatalf("the screen still says %q", got)
			}
		})
	}
}

func TestFinalRendererFailureCannotAnnounceRunSuccess(t *testing.T) {
	started := make(chan *agentBridge, 1)
	host, stop := runAgent(t, captureRuns{started})
	defer stop()
	host.Type("verify final document")
	host.Press(input.Enter)
	bridge := <-started
	failure := errors.New("final projection failed")
	if err := bridge.dispatch.Invoke(t.Context(), func() error {
		a := bridge.owner
		a.conversation.render = func(context.Context, string) (grid.Drawable, error) {
			return nil, failure
		}
		if err := a.conversation.Markdown(t.Context(), "retained source"); err != nil {
			return err
		}
		a.finishRun(nil)
		label := a.outcome.label(a.theme, a.glyphs)
		if a.outcome.kind != outcomeFailed || !strings.Contains(label.Text, failure.Error()) || a.status.Doing != "" {
			t.Errorf("failed final renderer produced outcome %+v and activity %q", a.outcome, a.status.Doing)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestSourceErrorDoesNotPoisonTheNextRun(t *testing.T) {
	started := make(chan *agentBridge, 2)
	host, stop := runAgent(t, captureRuns{started})
	defer stop()
	host.Type("first")
	host.Press(input.Enter)
	first := <-started
	if err := first.dispatch.Invoke(t.Context(), func() error {
		a := first.owner
		a.accept(program.ByteBatch{Data: make([]byte, (1<<20)+1)})
		if a.run != nil || a.outcome.kind != outcomeFailed || a.status.Doing != "" {
			t.Errorf("oversized first run: outcome=%+v activity=%q", a.outcome, a.status.Doing)
		}
		a.startRun("independent next run")
		if a.run == nil || a.outcome != (sessionOutcome{}) {
			t.Errorf("previous failure leaked to next run: outcome=%+v activity=%q", a.outcome, a.status.Doing)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
