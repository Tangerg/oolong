package kit

import (
	"strings"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
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
	// Text is what a row says. A tree given none draws nothing but its marks, which
	// is what an item that cannot be read as text comes to.
	Text func(item T) string
	// Theme is the look, and Glyphs are the marks beside a branch. A tree given no
	// glyphs draws no marks, and its rows are then told apart by their indent alone.
	Theme  Theme
	Glyphs Glyphs
	// Indent is how many columns a level is worth. Zero uses two.
	Indent int

	controller *headless.Tree[T]
}

// NewTree dresses controller with the kit tree appearance.
func NewTree[T any](
	theme Theme,
	glyphs Glyphs,
	controller *headless.Tree[T],
	read func(item T) string,
) *Tree[T] {
	return &Tree[T]{Theme: theme, Glyphs: glyphs, Text: read, controller: controller}
}

// Controller returns the headless tree that owns hierarchy and selection state.
func (t Tree[T]) Controller() *headless.Tree[T] { return t.controller }

// Handle passes the event to the tree — see [headless.Tree.Handle].
//
// This and the two below are here so that a dressed tree is a widget like any other:
// something that can go in a container, take the keyboard from it, and answer what
// the container hands it. A look that could only be drawn would have to be wired up
// by hand wherever it was used.
func (t Tree[T]) Handle(ev input.Event) bool {
	if t.controller == nil {
		return false
	}
	return t.controller.Handle(ev)
}

// Focus takes the keyboard, or gives it up.
func (t Tree[T]) Focus(has bool) {
	if t.controller != nil {
		t.controller.Focus(has)
	}
}

// Focused reports whether the tree has the keyboard, which is what a row asks to
// decide how a selection nobody is typing at should look.
func (t Tree[T]) Focused() bool { return t.controller != nil && t.controller.Focused() }

// Measure is one row per row the tree is showing.
func (t Tree[T]) Measure(across int) int {
	if t.controller == nil {
		return 0
	}
	return t.controller.Measure(across)
}

// Draw paints the rows that fit.
func (t Tree[T]) Draw(v headless.Frame) {
	if t.controller == nil {
		return
	}
	t.controller.DrawRows(v, t.row)
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
	depth := max(row.Depth, 0)
	if depth > width/indent {
		return
	}
	x := depth * indent
	if x >= width {
		return
	}
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
		return strings.Repeat(" ", max(text.Width(t.Glyphs.Collapsed), 0))
	case row.Open:
		return t.Glyphs.Expanded
	default:
		return t.Glyphs.Collapsed
	}
}
