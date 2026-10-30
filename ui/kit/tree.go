package kit

import (
	"strings"

	"github.com/Tangerg/oolong/ui/core/scene"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/text"
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
type Tree[T any] struct {
	// Text is what a row says. A tree given none draws nothing but its marks, which
	// is what an item that cannot be read as text comes to. Text runs during drawing
	// and must be an observationally pure projection.
	Text       func(item T) string
	appearance appearance
	Glyphs     Glyphs
	// Indent is how many columns a level is worth. Zero uses two.
	Indent int

	controller *base.Tree[T]
}

// TreeConfig selects the initial hierarchy and its appearance.
type TreeConfig[T any] struct {
	Scope      *ThemeScope
	Appearance ThemeOverride
	Theme      Theme
	Glyphs     Glyphs
	// Nodes supplies the initial hierarchy owned by the controller.
	Nodes []base.Node[T]
	// Text projects an item into its row label. Nil produces an empty label. It runs
	// during drawing and must be observationally pure.
	Text func(item T) string
	// Indent is the columns per depth. Zero means two.
	Indent int
}

// NewTree composes one base tree with its default appearance.
// It panics when Theme and Scope both declare palette sources, or when its base configuration is invalid.
func NewTree[T any](config TreeConfig[T]) *Tree[T] {
	look, appearanceErr := newAppearance(config.Theme, config.Scope, config.Appearance)
	if appearanceErr != nil {
		panic(appearanceErr)
	}
	tree := &Tree[T]{appearance: look, Glyphs: config.Glyphs, Text: config.Text, Indent: config.Indent, controller: base.NewTree(config.Nodes...)}
	if err := tree.controller.Element().SetPaint(tree.Draw); err != nil {
		panic(err)
	}
	tree.appearance.bind(tree.Element())
	return tree
}

// Controller returns the base tree that owns hierarchy and selection state.
func (t *Tree[T]) Controller() *base.Tree[T] {
	if t == nil {
		return nil
	}
	return t.controller
}

// HeightForWidth is one row per row the tree is showing.
func (t *Tree[T]) HeightForWidth(across int) int {
	if t == nil || t.controller == nil {
		return 0
	}
	return t.controller.HeightForWidth(across)
}

// Draw paints the rows that fit.
func (t *Tree[T]) Draw(v scene.Frame) {
	if t == nil || t.controller == nil {
		return
	}
	t.controller.DrawRows(v, t.row)
}

func (t *Tree[T]) row(v grid.View, _ int, row base.Shown[T], selected bool) {
	width, _ := v.Size()
	if width <= 0 {
		return
	}
	style := t.appearance.palette().Text
	if selected {
		style = t.appearance.palette().Text.Merge(t.appearance.palette().Selection)
		v.Fill(grid.Area(0, 0, width, 1), t.appearance.palette().Selection)
	}

	// The default belongs to the zero value, not to every width below it: a caller
	// who asked for one column meant one.
	indent := t.Indent
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
		x = layout.Sum(x, v.Text(x, 0, mark, t.appearance.palette().Subtle.Merge(style)), 1)
	}
	if t.Text == nil || x >= width {
		return
	}
	v.Text(x, 0, text.Truncate(t.Text(row.Item), width-x, t.Glyphs.Ellipsis), style)
}

// mark is what goes before a row: which way a branch is turned, or a space as wide
// as one so that leaves line up with their siblings.
func (t *Tree[T]) mark(row base.Shown[T]) string {
	switch {
	case !row.Branch:
		return strings.Repeat(" ", max(text.Width(t.Glyphs.Collapsed), 0))
	case row.Open:
		return t.Glyphs.Expanded
	default:
		return t.Glyphs.Collapsed
	}
}

// Element returns the controller's one scene identity with its kit appearance installed.
func (t *Tree[T]) Element() scene.Element {
	if t == nil || t.controller == nil {
		return scene.Element{}
	}
	return t.controller.Element()
}
