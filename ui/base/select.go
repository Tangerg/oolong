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

// Option is one thing a choice offers.
type Option[T any] struct {
	Label string
	Value T
}

// Equal compares values by Go equality. Use Equal[T] as Same for comparable choices.
func Equal[T comparable](a, b T) bool { return a == b }

func (o Option[T]) holds(want T, same func(a, b T) bool) bool {
	return same(o.Value, want)
}

// Options is the usual case, where what is shown is what it means.
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
// The zero value is empty and ready. A Select must not be copied after first use: its
// options, local cursor and scroll are one mutable field.
// Initial selects an available option; later transitions use Choose.
type Select[T any] struct {
	noCopy noCopy

	field
	Label string
	// Same defines value identity, independently of Label. It is required before
	// installing non-empty options. Use Equal[T] for comparable values, or supply
	// domain equality for arbitrary T. It must be pure and remain stable while options
	// are installed; changing the identity rule requires constructing a new field.
	Same func(a, b T) bool
	// Check says what is wrong with the choice, or nil.
	Check func(v T) error
	// Rows caps how many options are shown at once. Zero shows them all.
	Rows int
	// Keys say which keystrokes move the cursor. Nil reads through [DefaultListKeys].
	Keys *keymap.Map

	// Row optionally draws a one-row choice with its cursor and taken states.
	// Nil uses Look's default choice row. Like Draw, the callback must be pure.
	Row func(grid.View, Option[T], bool, bool, Look)

	list     List[Option[T]]
	matcher  keymap.Matcher
	revision uint64
}

func (s *Select[T]) Prompt() string { return s.Label }

// SetOptions replaces what is on offer. Select owns the slice. If the selected
// choice still exists under Same, the cursor follows it to its new position.
// Otherwise the cursor is clamped. Non-empty options require Same; omitting it
// is a programmer error and panics.
// Required reads and identity comparisons finish before options or selection change.
// An abnormal exit during preparation preserves the previous field.
func (s *Select[T]) SetOptions(options []Option[T]) {
	requireMutable(s.element)
	if len(options) > 0 && s.Same == nil {
		panic("base: Select requires Same before SetOptions")
	}
	previous, exists := s.Chosen()
	at := s.list.Selected()
	changed := exists != (len(options) != 0)
	if exists && len(options) != 0 {
		found, retained := s.indexOf(options, previous.Value)
		changed = !retained
		if retained {
			at = found
		}
	}
	s.matcher.Reset(func() {
		s.list.replaceItems(options, at)
		for i := range s.list.items {
			s.list.items[i].Label = strings.Clone(s.list.items[i].Label)
		}
		if changed {
			s.revision++
		}
		s.invalidateValidation()
		s.list.notifyChange()
	})
}

// Options returns a copy of what is on offer.
func (s *Select[T]) Options() []Option[T] { return slices.Clone(s.list.items) }

// Chosen reads the current choice.
func (s *Select[T]) Chosen() (Option[T], bool) {
	return s.list.At(s.selected())
}

func (s *Select[T]) selected() int { return s.list.Selected() }

func (s *Select[T]) HeightForWidth(int) int {
	rows := len(s.list.items)
	if s.Rows > 0 {
		rows = min(rows, s.Rows)
	}
	return layout.Sum(max(rows, 1), s.rows(s.Label))
}

func (s *Select[T]) Draw(v scene.Frame) {
	s.DrawWith(v, Look{})
}

func (s *Select[T]) DrawWith(v scene.Frame, look Look) {
	s.list.DrawRows(s.frame(v, s.Label, look), func(v grid.View, _ int, option Option[T], under bool) {
		if s.Row != nil {
			s.Row(v, option, under, under, look)
		} else {
			look.choice(v, option.Label, under, under)
		}
	})
}

func optionIndex[T any](options []Option[T], value T, same func(T, T) bool) int {
	return slices.IndexFunc(options, func(option Option[T]) bool { return option.holds(value, same) })
}

func (s *Select[T]) indexOf(options []Option[T], value T) (int, bool) {
	at := readProjection(s.Element(), func() int { return optionIndex(options, value, s.Same) })
	return at, at >= 0
}

// Handle moves the cursor, and takes the choice with it.
func (s *Select[T]) Handle(ev input.Event) bool {
	requireMutable(s.element)
	if key, ok := ev.(input.Key); ok {
		keys := s.Keys
		if keys == nil {
			keys = listKeys()
		}
		_, handled := s.matcher.Handle(keys, key, s.Do)
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

func (s *Select[T]) checkAnswer() error {
	if s.Check == nil {
		return nil
	}
	chosen, _ := s.Chosen()
	return s.Check(chosen.Value)
}

func (s *Select[T]) focusChanged(has bool) {
	change := func() {
		s.focus(has, s.list.focusChanged, nil, s.Validate)
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
	before, ok := s.Chosen()
	next, found := s.list.At(at)
	if ok && found && s.selected() != at {
		if !readProjection(s.Element(), func() bool { return next.holds(before.Value, s.Same) }) {
			s.revision++
		}
	}
	s.list.Select(at)
}

// Revision identifies a semantic value version; view-only navigation does not change it.
func (s *Select[T]) Revision() uint64 { return s.revision }
