package base

import (
	"slices"
	"strings"

	"github.com/Tangerg/oolong/ui/core/scene"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/keymap"
	"github.com/Tangerg/oolong/ui/core/layout"
)

// Option values may contain references; copying an option does not copy its domain data.
// Keep that data immutable while installed and publish replacements with SetOptions
// so validation and submission receipts describe the complete answer.
type Option[T any] struct {
	Label string
	Value T
}

// Equal can supply Same when Go equality defines the choice's identity.
func Equal[T comparable](a, b T) bool { return a == b }

func Options[T ~string](values ...T) []Option[T] {
	out := make([]Option[T], len(values))
	for i, v := range values {
		out[i] = Option[T]{Label: string(v), Value: v}
	}
	return out
}

// Select is a field holding one choice out of several.
//
// The choice follows the cursor: what is under it is what is chosen, and there is
// nothing to press to confirm. A list that made somebody move to a row and then take it
// is a list that can be left on a row nobody took.
//
// The zero value is empty; non-empty options require NewSelect with Same.
// A Select must not be copied after first use: its options, local cursor and scroll
// are one mutable field.
// Initial selects an available option; later transitions use Choose.
type Select[T any] struct {
	noCopy noCopy

	field
	label string
	// Rows caps how many options are shown at once. Zero shows them all.
	rowLimit int
	// Keys say which keystrokes move the cursor. Nil reads through [DefaultListKeys].
	keys keyBinding

	// Row optionally draws a one-row choice with its cursor and taken states.
	// Nil uses Look's default choice row. Like Draw, the callback must be pure.
	rowPainter func(grid.View, Option[T], bool, bool, Look)

	list         List[Option[T]]
	matcher      keymap.Matcher
	sameIdentity func(T, T) bool
	check        func(T) error
	revision     uint64
}

func (s *Select[T]) Prompt() string { return s.label }

// SetOptions replaces what is on offer. Select owns the slice. If the selected
// choice still exists under Same, the cursor follows it to its new position.
// Otherwise the cursor is clamped. A zero-value Select panics on non-empty
// options; construct one with Same instead.
// Replacing an available answer advances Revision even when its identity is retained:
// identity does not establish that the complete payload is unchanged.
// Required reads and identity comparisons finish before options or selection change.
// An abnormal exit during preparation preserves the previous field.
func (s *Select[T]) SetOptions(options []Option[T]) {
	requireMutable(s.element)
	if len(options) > 0 && s.sameIdentity == nil {
		panic("base: Select requires Same in its construction config")
	}
	previous, exists := s.Chosen()
	at := s.list.Selected()
	replacesAnswer := exists || len(options) != 0
	if exists && len(options) != 0 {
		found, retained := s.indexOf(options, previous.Value)
		if retained {
			at = found
		}
	}
	s.matcher.Reset(func() {
		s.list.replaceItems(options, at)
		for i := range s.list.items {
			s.list.items[i].Label = strings.Clone(s.list.items[i].Label)
		}
		if replacesAnswer {
			s.revision++
		}
		s.invalidateValidation()
		s.list.notifyChange()
	})
}

// Options returns a copy of what is on offer.
func (s *Select[T]) Options() []Option[T] { return slices.Clone(s.list.items) }

func (s *Select[T]) Chosen() (Option[T], bool) {
	return s.list.At(s.selected())
}

func (s *Select[T]) selected() int { return s.list.Selected() }

func (s *Select[T]) HeightForWidth(int) int {
	rows := len(s.list.items)
	if s.rowLimit > 0 {
		rows = min(rows, s.rowLimit)
	}
	return layout.Sum(max(rows, 1), s.rows(s.label))
}

func (s *Select[T]) Draw(v scene.Frame) {
	s.DrawWith(v, Look{})
}

func (s *Select[T]) DrawWith(v scene.Frame, look Look) {
	s.list.DrawRows(s.frame(v, s.label, look), func(v grid.View, _ int, option Option[T], under bool) {
		if s.rowPainter != nil {
			s.rowPainter(v, option, under, under, look)
		} else {
			look.choice(v, option.Label, under, under)
		}
	})
}

func optionIndex[T any](options []Option[T], value T, sameIdentity func(T, T) bool) int {
	return slices.IndexFunc(options, func(option Option[T]) bool { return sameIdentity(option.Value, value) })
}

func (s *Select[T]) indexOf(options []Option[T], value T) (int, bool) {
	at := readProjection(s.Element(), func() int { return optionIndex(options, value, s.sameIdentity) })
	return at, at >= 0
}

func (s *Select[T]) Handle(ev input.Event) bool {
	requireMutable(s.element)
	if key, ok := ev.(input.Key); ok {
		keys := s.keys.current
		if keys == nil {
			keys = listKeys()
		}
		_, handled := s.matcher.Handle(keys, key, keyboardReceipt(&s.element), s.Do)
		return handled
	}
	if mouse, ok := ev.(input.Mouse); ok {
		return s.list.mouse(s.local(mouse), s.selectAt)
	}
	return false
}

func (s *Select[T]) Do(action keymap.Action) bool {
	return s.list.navigate(action, s.selectAt)
}

func (s *Select[T]) Validate() error {
	return s.validate(nil, s.Revision, s.checkAnswer)
}

// SetCheck revokes old results because validation belongs to the rule as well as
// the answer. Reinstalling the same callback also revokes receipts; nil removes it.
func (s *Select[T]) SetCheck(check func(T) error) {
	requireMutable(s.element)
	s.check = check
	s.InvalidateValidation()
}

func (s *Select[T]) checkAnswer() error {
	if s.check == nil {
		return nil
	}
	chosen, _ := s.Chosen()
	return s.check(chosen.Value)
}

func (s *Select[T]) focusChanged(has bool) {
	change := func() {
		s.focus(has, s.list.focusChanged, s.Validate)
	}
	if has {
		change()
	} else {
		s.matcher.Reset(change)
	}
}

// Choose selects an available value using the configured identity rule.
func (s *Select[T]) Choose(value T) bool {
	at, ok := s.indexOf(s.list.items, value)
	if !ok {
		return false
	}
	s.selectAt(at)
	return true
}

func (s *Select[T]) selectAt(at int) {
	requireMutable(s.element)
	if s.list.clampIndex(at) != s.list.selected {
		s.revision++
	}
	s.list.Select(at)
}

// Revision identifies the selected option's answer snapshot. A different option or
// a replacement collection with an available answer advances it conservatively;
// Same cannot establish equality of arbitrary payloads.
func (s *Select[T]) Revision() uint64 { return s.revision }

func (s *Select[T]) Label() string { return s.label }

func (s *Select[T]) SetLabel(value string) {
	element := s.Element()
	requireMutable(element)
	if s.label == value {
		return
	}
	s.label = strings.Clone(value)
	element.Invalidate()
}

func (s *Select[T]) Rows() int { return s.rowLimit }

// SetRows panics for a negative limit. Zero permits all options to contribute to height.
func (s *Select[T]) SetRows(value int) {
	element := s.Element()
	requireMutable(element)
	if value < 0 {
		panic("base: Rows cannot be negative")
	}
	if s.rowLimit == value {
		return
	}
	s.rowLimit = value
	element.Invalidate()
}

func (s *Select[T]) Row() func(grid.View, Option[T], bool, bool, Look) { return s.rowPainter }

func (s *Select[T]) SetRow(value func(grid.View, Option[T], bool, bool, Look)) {
	element := s.Element()
	requireMutable(element)
	s.rowPainter = value
	element.Invalidate()
}

func (s *Select[T]) Keys() *keymap.Map { return s.keys.current }

func (s *Select[T]) SetKeys(keys *keymap.Map) {
	s.keys.set(s.Element(), &s.matcher, keys)
}
