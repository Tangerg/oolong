package kit

import (
	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/text"
)

// treeIndent is how many columns a level is worth when nothing else is said. Two:
// one is not a step and four runs out of width three levels down.
const treeIndent = 2

// Tree draws the rows of a [headless.Tree] with an indent and a mark on what can be
// opened.
//
// It is one answer to the question that behaviour refuses — what a branch looks like
// — and the answer is: as far in as it is deep, a mark that turns over when it
// opens, and the row under the cursor drawn as a selection.
type Tree[T any] struct {
	// Of is the tree being shown. Nil draws nothing.
	Of *headless.Tree[T]
	// Text is what a row says. A tree given none draws nothing but its marks, which
	// is what an item that cannot be read as text comes to.
	Text func(item T) string
	// Theme is the look, and Glyphs are the marks beside a branch. A tree given no
	// glyphs draws no marks, and its rows are then told apart by their indent alone.
	Theme  Theme
	Glyphs Glyphs
	// Indent is how many columns a level is worth. Zero uses two.
	Indent int
}

// Measure is one row per row the tree is showing.
func (t Tree[T]) Measure(across int) int {
	if t.Of == nil {
		return 0
	}
	return t.Of.Measure(across)
}

// Draw paints the rows that fit.
func (t Tree[T]) Draw(v grid.View) {
	if t.Of == nil {
		return
	}
	t.Of.Row = t.row
	t.Of.Draw(v)
}

// row draws one row: the indent, the mark, and what the item says.
func (t Tree[T]) row(v grid.View, _ int, row headless.Shown[T], selected bool) {
	width, _ := v.Size()
	if width <= 0 {
		return
	}
	style := t.Theme.Text
	if selected {
		style = t.Theme.Text.Merge(t.Theme.Selection)
		v.Fill(grid.Rect(0, 0, width, 1), t.Theme.Selection)
	}

	indent := max(t.Indent, treeIndent)
	x := row.Depth * indent
	if mark := t.mark(row); mark != "" {
		x += v.Text(x, 0, mark, t.Theme.Subtle.Merge(style)) + 1
	}
	if t.Text == nil || x >= width {
		return
	}
	v.Text(x, 0, text.Truncate(t.Text(row.Item), width-x, t.Glyphs.Ellipsis), style)
}

// mark is what goes before a row: which way a branch is turned, or a space as wide
// as one so that leaves line up with their siblings.
func (t Tree[T]) mark(row headless.Shown[T]) string {
	switch {
	case !row.Branch:
		// A leaf beside a branch has to start in the same column, or a tree of files
		// reads as a tree of two different things.
		return blanks(text.Width(t.Glyphs.Collapsed))
	case row.Open:
		return t.Glyphs.Expanded
	default:
		return t.Glyphs.Collapsed
	}
}

// blanks is n columns of nothing, and nothing at all for a set with no marks in it.
func blanks(n int) string {
	if n <= 0 {
		return ""
	}
	out := make([]byte, n)
	for i := range out {
		out[i] = ' '
	}
	return string(out)
}
