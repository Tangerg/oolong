package main

import (
	"strings"
	"testing"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
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
	c.Markdown(t.Context(), "<")
	c.Markdown(t.Context(), "!-- hidden -->")
	c.FlushMarkdown(t.Context())
	if c.item.State() == base.ContentLive {
		t.Fatal("the answer remained live after EOF")
	}
	first := c.item
	for range 20 {
		c.Markdown(t.Context(), "a line\n\n")
		c.FlushMarkdown(t.Context())
	}
	if c.Err() != nil {
		t.Fatal(c.Err())
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
	c.User(t.Context(), strings.Repeat("a long prompt that fills the window\n", 20))
	c.Markdown(t.Context(), "first\n\n<!-- separator -->\n\n")
	draw(t, c, 6)
	c.content.Scroll().ToTop()
	if c.content.Scroll().FollowingEnd() {
		t.Fatal("the reader is at the end after scrolling to the top")
	}

	c.Markdown(t.Context(), "second\n\nthird\n")
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
			c.Markdown(t.Context(), "<")
			draw(t, c, 8)
			c.Markdown(t.Context(), "!-- hidden -->")
			if settle == "flushed" {
				c.FlushMarkdown(t.Context())
			}
			if got := drawn(t, c, 8); strings.Contains(got, "<") {
				t.Fatalf("the screen still says %q", got)
			}
		})
	}
}
