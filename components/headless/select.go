package headless

import (
	"slices"
	"strings"

	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/keymap"
	"github.com/Tangerg/oolong/core/layout"
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
// NewSelect configures a caller-owned answer; SetBinding transfers ownership.
// A controlled cursor projects the accessor's accepted choice. An unavailable answer
// projects the first option and is offered back to the owner during validation.
type Select[T any] struct {
	noCopy noCopy

	field
	value Accessor[T]
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

	list    List[Option[T]]
	matcher keymap.Matcher
}

// SetBinding transfers the answer to a new accessor and invalidates old validation
// and key sequences. Nil retains the currently accepted answer as local state.
// The completed transfer is visible to resolver cancellation callbacks.
// Required reads and identity comparisons finish before the transfer. An abnormal
// exit preserves the previous owner and validation.
func (s *Select[T]) SetBinding(value Accessor[T]) {
	var selected int
	if value == nil {
		selected = s.selected()
	} else {
		selected, _ = s.indexOf(s.list.items, value.Value())
	}
	s.matcher.Reset(func() {
		s.value = value
		if value == nil {
			s.list.Select(selected)
		}
		s.list.reveal(selected)
		s.invalidateValidation()
	})
}

// Prompt is what the field is asking for.
func (s *Select[T]) Prompt() string { return s.Label }

// SetOptions replaces what is on offer. Select owns the slice. If the selected
// choice still exists under Same, the cursor follows it to its new position.
// Otherwise a local cursor is clamped; a controlled field offers the first option
// to its owner. Non-empty options require Same; omitting it
// is a programmer error and panics. The initial unmatched value is settled by Validate.
// Required reads and identity comparisons finish before options or selection change.
// An abnormal exit during preparation preserves the previous field.
func (s *Select[T]) SetOptions(options []Option[T]) {
	previous, hadPrevious := s.list.Current()
	if len(options) > 0 && s.Same == nil {
		panic("headless: Select requires Same before SetOptions")
	}
	selected := s.list.Selected()
	binding := s.value
	normalize := false
	if hadPrevious && len(options) > 0 {
		if binding == nil {
			if at, found := s.indexOf(options, previous.Value); found {
				selected = at
			}
		} else {
			_, found := s.indexOf(options, binding.Value())
			normalize = !found
		}
	}
	s.matcher.Reset(func() {
		if binding == nil {
			s.list.SetItems(options)
			s.list.Select(selected)
		} else {
			s.list.replaceItems(options)
		}
		for i := range s.list.items {
			s.list.items[i].Label = strings.Clone(s.list.items[i].Label)
		}
		if normalize {
			binding.Set(s.list.items[0].Value)
		}
	})
}

// Options returns a copy of what is on offer.
func (s *Select[T]) Options() []Option[T] { return slices.Clone(s.list.items) }

// Chosen projects the current choice without changing the cursor or writing Value.
func (s *Select[T]) Chosen() (Option[T], bool) {
	return s.list.At(s.selected())
}

func (s *Select[T]) selected() int {
	if s.value != nil {
		at, _ := s.indexOf(s.list.items, s.value.Value())
		return at
	}
	return s.list.Selected()
}

// HeightForWidth is the label, the options within their cap, and the problem if there is one.
func (s *Select[T]) HeightForWidth(int) int {
	rows := len(s.list.items)
	if s.Rows > 0 {
		rows = min(rows, s.Rows)
	}
	return layout.Sum(max(rows, 1), s.rows(s.Label))
}

// Draw paints the label, the options and whatever was wrong with the choice.
func (s *Select[T]) Draw(v Frame) {
	s.DrawWith(v, Look{})
}

// DrawWith paints the field using this frame's look without changing configuration.
func (s *Select[T]) DrawWith(v Frame, look Look) {
	var choice *int
	if s.value != nil {
		selected := s.selected()
		choice = &selected
	}
	s.list.drawRows(s.frame(v, s.Label, look), choice, func(v grid.View, _ int, option Option[T], under bool) {
		if s.Row != nil {
			s.Row(v, option, under, under, look)
		} else {
			look.choice(v, option.Label, under, under)
		}
	})
}

func (s *Select[T]) indexOf(options []Option[T], want T) (int, bool) {
	for i, option := range options {
		if option.holds(want, s.Same) {
			return i, true
		}
	}
	return 0, false
}

// Handle moves the cursor, and takes the choice with it.
func (s *Select[T]) Handle(ev input.Event) bool {
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

// Do runs one of the field's actions by name. See [Doer].
func (s *Select[T]) Do(action keymap.Action) bool {
	before := s.selected()
	at, known := s.list.target(action, before)
	if !known {
		return false
	}
	if at == before {
		s.list.reveal(before)
	} else {
		s.selectAt(at)
	}
	return true
}

// Validate checks the choice.
func (s *Select[T]) Validate() error {
	return s.validate(func() error {
		// An unmatched initial value is settled at this semantic boundary, once.
		s.selectAt(s.selected())
		return nil
	}, s.checkAnswer)
}

func (s *Select[T]) checkAnswer() fieldCheck {
	chosen, ok := s.Chosen()
	var answer T
	if ok {
		answer = chosen.Value
	}
	checked := fieldCheck{current: func() bool {
		current, exists := s.Chosen()
		if exists != ok || ok && !current.holds(answer, s.Same) {
			return false
		}
		if s.value != nil {
			_, matched := s.indexOf(s.list.items, s.value.Value())
			return matched
		}
		return true
	}}
	if !checked.current() {
		return fieldCheck{err: ErrInvalidBinding}
	}
	if s.Check != nil {
		checked.err = s.Check(answer)
	}
	return checked
}

// Focus takes the keyboard or gives it up, and checks the choice on the way out.
func (s *Select[T]) Focus(has bool) {
	change := func() {
		s.focus(has, s.list.Focus, nil, s.Validate)
	}
	if has {
		change()
	} else {
		s.matcher.Reset(change)
	}
}

func (s *Select[T]) selectAt(at int) {
	if s.value == nil {
		s.list.Select(at)
		return
	}
	if chosen, ok := s.list.At(at); ok {
		if chosen.holds(s.value.Value(), s.Same) {
			return
		}
		s.value.Set(chosen.Value)
	}
}
