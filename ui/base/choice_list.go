package base

import (
	"slices"
	"strings"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/keymap"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/scene"
)

// choiceList is what Select and MultiSelect share: a labelled list of options
// under one identity rule, with its keys, row limit and row painter. Each field
// adds only what its answer is and how a row shows it.
type choiceList[T any] struct {
	field
	label string
	// rowLimit caps how many options are shown at once. Zero shows them all.
	rowLimit int
	// keys say which keystrokes move the cursor and, for a MultiSelect, take.
	// Nil reads through the field's defaults.
	keys keyBinding
	// rowPainter optionally draws a one-row choice with its cursor and taken
	// states. Nil uses Look's default choice row. Like Draw, it must be pure.
	rowPainter   func(grid.View, Option[T], bool, bool, Look)
	list         List[Option[T]]
	matcher      keymap.Matcher
	sameIdentity func(T, T) bool
	revision     uint64
}

func (c *choiceList[T]) initialize(label string, same func(T, T) bool, rows int, keys *keymap.Map, row func(grid.View, Option[T], bool, bool, Look)) {
	c.label, c.sameIdentity, c.rowLimit = strings.Clone(label), same, rows
	c.keys, c.rowPainter = keyBinding{current: keys}, row
}

// adopt makes element the identity of the field and of the list it draws with.
func (c *choiceList[T]) adopt(element scene.Element) {
	c.element = element
	c.list.element = element
	c.list.scroll.element = element
	c.keys.attach(element, &c.matcher)
}

func (c *choiceList[T]) Prompt() string { return c.label }

func (c *choiceList[T]) Label() string { return c.label }

func (c *choiceList[T]) SetLabel(value string) {
	requireMutable(c.element)
	if c.label == value {
		return
	}
	c.label = strings.Clone(value)
	c.element.Invalidate()
}

// Options returns a copy of what is on offer.
func (c *choiceList[T]) Options() []Option[T] { return slices.Clone(c.list.items) }

func (c *choiceList[T]) Rows() int { return c.rowLimit }

// SetRows panics for a negative limit. Zero permits all options to contribute to height.
func (c *choiceList[T]) SetRows(value int) {
	requireMutable(c.element)
	requireNonNegative("choice Rows", value)
	if c.rowLimit == value {
		return
	}
	c.rowLimit = value
	c.element.Invalidate()
}

func (c *choiceList[T]) Row() func(grid.View, Option[T], bool, bool, Look) { return c.rowPainter }

func (c *choiceList[T]) SetRow(value func(grid.View, Option[T], bool, bool, Look)) {
	requireMutable(c.element)
	c.rowPainter = value
	c.element.Invalidate()
}

func (c *choiceList[T]) Keys() *keymap.Map { return c.keys.current }

func (c *choiceList[T]) SetKeys(keys *keymap.Map) {
	c.keys.set(c.element, &c.matcher, keys)
}

func (c *choiceList[T]) HeightForWidth(int) int {
	rows := len(c.list.items)
	if c.rowLimit > 0 {
		rows = min(rows, c.rowLimit)
	}
	return layout.Sum(max(rows, 1), c.rows(c.label))
}

// drawRows paints the visible options; taken reports whether an option is part
// of the answer.
func (c *choiceList[T]) drawRows(v scene.Frame, look Look, taken func(option Option[T], under bool) bool) {
	c.list.DrawRows(c.frame(v, c.label, look), func(v grid.View, _ int, option Option[T], under bool) {
		chosen := taken(option, under)
		if c.rowPainter != nil {
			c.rowPainter(v, option, under, chosen, look)
		} else {
			look.choice(v, option.Label, under, chosen)
		}
	})
}

func (c *choiceList[T]) focusChanged(has bool, validate func() error) {
	change := func() {
		c.focus(has, c.list.focusChanged, validate)
	}
	if has {
		change()
	} else {
		c.matcher.Reset(change)
	}
}

// Revision identifies the answer snapshot. A different answer or a replacement
// collection with an available answer advances it conservatively; Same cannot
// establish equality of arbitrary payloads.
func (c *choiceList[T]) Revision() uint64 { return c.revision }
