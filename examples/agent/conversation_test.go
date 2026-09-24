package main

import (
	"strings"
	"testing"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
)

// draw gives the conversation a width, which is what everything it owns is measured
// against, and a height, which is what decides whether it is scrolled.
func draw(t *testing.T, c *conversation, height int) {
	t.Helper()
	headless.NewRoot(c).Draw(grid.NewSurface(40, height).View())
}

func newTestConversation() *conversation {
	return newConversation(kit.Theme{}, kit.GlyphsFor(""), input.Wheel{})
}

// finishedBlocks is how many blocks from the front of the transcript have finished,
// which is where retention stops.
func finishedBlocks(c *conversation) int {
	finished := 0
	for i := range c.content.Len() {
		if !c.content.Finished(c.content.FirstBlock() + headless.BlockID(i)) {
			break
		}
		finished++
	}
	return finished
}

func TestAnAnswerThatEndsWithNothingLeftStillEnds(t *testing.T) {
	// Retention stops at the first block that has not finished, so a block left open
	// for good takes everything after it with it: the terminal's own scrollback never
	// receives another line of the session.
	c := newTestConversation()
	draw(t, c, 10)
	c.Markdown("<")
	c.Markdown("!-- hidden -->")
	c.FlushMarkdown()

	if got := finishedBlocks(c); got != c.content.Len() {
		t.Fatalf("%d of %d blocks finished after the answer ended",
			got, c.content.Len())
	}

	for range retainedAgentBlocks + 12 {
		c.Markdown("a line\n\n")
		c.FlushMarkdown()
	}
	draw(t, c, 10)
	var scrollback countingPrinter
	c.Retain(&scrollback)
	if scrollback.blocks == 0 {
		t.Fatal("nothing was ever handed to the terminal's own scrollback")
	}
}

// countingPrinter stands in for the terminal's own output.
type countingPrinter struct{ blocks int }

func (p *countingPrinter) Print(grid.Drawable) { p.blocks++ }

func TestMoreOfTheSameAnswerDoesNotTakeTheReaderBackToIt(t *testing.T) {
	// A reader who scrolled up is reading. An answer still being written arrives in a
	// great many pieces, and the piece that happens to arrive with no open block
	// behind it is no more a reason to move them than any of the others.
	c := newTestConversation()
	c.User(strings.Repeat("a long prompt that fills the window\n", 20))
	c.Markdown("first\n\n<!-- separator -->\n\n")
	draw(t, c, 6)
	c.scroll.ToTop()
	if c.scroll.FollowingEnd() {
		t.Fatal("the reader is at the end after scrolling to the top")
	}

	c.Markdown("second\n\nthird\n")
	draw(t, c, 6)
	if c.scroll.FollowingEnd() {
		t.Fatal("more of the same answer took the reader back to the end of it")
	}
}
