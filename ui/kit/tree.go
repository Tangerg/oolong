package kit

import (
	"strings"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/core/text"
	"github.com/Tangerg/oolong/ui/internal/setting"
)

// treeIndent is how many columns a level is worth when nothing else is said. Two:
// one is not a step and four runs out of width three levels down.
const treeIndent = 2

// Tree draws the rows of a [base.Tree] with an indent and a mark on what can be
// opened.
//
// It is one answer to the question that behaviour refuses — what a branch looks like
// — and the answer is: as far in as it is deep, a mark that turns over when it
// opens, and the row under the cursor drawn as a selection.
// A Tree must not be copied after first use: its controller retains this appearance.
type Tree[T any] struct {
	noCopy     noCopy
	text       func(item T) string
	appearance appearance
	glyphs     Glyphs
	indent     int

	controller *base.Tree[T]
}

// TreeConfig carries the base tree configuration in Behavior; the skin draws the
// rows, so Behavior.Row must be nil.
type TreeConfig[T any] struct {
	Behavior   base.TreeConfig[T]
	Scope      *ThemeScope
	Appearance ThemeOverride
	Theme      Theme
	Glyphs     Glyphs
	// Text projects an item into its row label. Nil produces an empty label. It runs
	// during drawing and must be observationally pure.
	Text func(item T) string
	// Indent is the columns per depth. Zero means two.
	Indent int
}

// NewTree panics for negative Indent, cyclic node collections, a row painter in
// Behavior, or competing Theme and Scope palette sources.
func NewTree[T any](config TreeConfig[T]) *Tree[T] {
	look := newAppearance(config.Theme, config.Scope, config.Appearance)
	requireSkinOwned("tree Behavior.Row", config.Behavior.Row == nil)
	tree := &Tree[T]{appearance: look, glyphs: config.Glyphs.clone(), text: config.Text, controller: base.NewTree(config.Behavior)}
	tree.SetIndent(config.Indent)
	tree.controller.Element().SetPaint(tree.Draw)
	tree.appearance.bind(tree.Element())
	return tree
}

func (t *Tree[T]) Controller() *base.Tree[T] {
	return t.controller
}

func (t *Tree[T]) HeightForWidth(across int) int {
	return t.controller.HeightForWidth(across)
}

func (t *Tree[T]) Draw(v scene.Frame) {
	t.controller.DrawRows(v, t.row)
}

func (t *Tree[T]) row(v grid.View, _ int, row base.Shown[T], selected bool) {
	width, _ := v.Size()
	if width <= 0 {
		return
	}
	style := t.appearance.rowStyle(v, selected)

	// The default belongs to the zero value, not to every width below it: a caller
	// who asked for one column meant one.
	indent := t.indent
	if indent <= 0 {
		indent = treeIndent
	}
	depth := max(row.Depth, 0)
	if depth > width/indent {
		return
	}
	x := depth * indent
	if x >= width {
		return
	}
	if mark := t.mark(row); mark != "" {
		x = layout.Sum(x, text.Of(mark, t.appearance.palette().Subtle.Merge(style)).Draw(v, x, 0), 1)
	}
	if t.text == nil || x >= width {
		return
	}
	text.Of(t.text(row.Item), style).Truncate(width-x, t.glyphs.Ellipsis).Draw(v, x, 0)
}

// mark is what goes before a row: which way a branch is turned, padded to the
// wider of the two turns so open branches, closed branches and leaves all start
// their text in the same column.
func (t *Tree[T]) mark(row base.Shown[T]) string {
	mark := ""
	switch {
	case row.Branch && row.Open:
		mark = t.glyphs.Expanded
	case row.Branch:
		mark = t.glyphs.Collapsed
	}
	column := max(text.Width(t.glyphs.Expanded), text.Width(t.glyphs.Collapsed))
	if column == 0 {
		return ""
	}
	return mark + strings.Repeat(" ", column-text.Width(mark))
}

func (t *Tree[T]) Element() scene.Element {
	return t.controller.Element()
}

func (t *Tree[T]) Text() func(item T) string { return t.text }

// SetText panics with scene.ErrBusy outside owner mutation: retained
// presentation must stay stable during observation and frame preparation.
func (t *Tree[T]) SetText(value func(item T) string) {
	setting.Replace(t.Element(), &t.text, value)
}

func (t *Tree[T]) Glyphs() Glyphs { return t.glyphs.clone() }

// SetGlyphs panics with scene.ErrBusy outside owner mutation: retained
// presentation must stay stable during observation and frame preparation.
func (t *Tree[T]) SetGlyphs(value Glyphs) { setting.Replace(t.Element(), &t.glyphs, value.clone()) }

func (t *Tree[T]) Indent() int { return t.indent }

// SetIndent panics for negative indentation or with scene.ErrBusy outside
// owner mutation, before the retained presentation can change.
func (t *Tree[T]) SetIndent(value int) {
	if value < 0 {
		panic("kit: Indent must be non-negative")
	}
	setting.Assign(t.Element(), &t.indent, value)
}
