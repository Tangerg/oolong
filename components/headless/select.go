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
	// Label is the row as shown.
	Label string
	// Value is what choosing it means.
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
// options, cursor, scroll and controlled-state settlement are one mutable field.
// NewSelect configures a caller-owned answer; SetBinding transfers ownership.
// The cursor follows the accessor's accepted choice. An unavailable answer projects
// the current cursor and is offered back to the owner during validation.
type Select[T any] struct {
	noCopy noCopy

	field
	value Accessor[T]
	// Label is what the field is asking for.
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
func (s *Select[T]) SetBinding(value Accessor[T]) {
	s.matcher.Reset(func() {
		previous, hadPrevious := s.Chosen()
		s.value = value
		if value == nil && hadPrevious {
			for i, option := range s.list.items {
				if option.holds(previous.Value, s.Same) {
					s.list.Select(i)
					break
				}
			}
		} else {
			s.sync()
		}
		s.invalidateValidation()
	})
}

// Prompt is what the field is asking for.
func (s *Select[T]) Prompt() string { return s.Label }

// SetOptions replaces what is on offer. Select owns the slice. If the selected
// choice still exists under Same, the cursor
// follows it to its new position; otherwise the cursor is clamped and the bound
// value follows the resulting choice. Non-empty options require Same; omitting it
// is a programmer error and panics. The initial unmatched value is settled by Validate.
func (s *Select[T]) SetOptions(options []Option[T]) {
	s.matcher.Reset(func() { s.setOptions(options) })
}

func (s *Select[T]) setOptions(options []Option[T]) {
	previous, hadPrevious := s.list.Current()
	if hadPrevious && s.value != nil {
		previous, hadPrevious = s.Chosen()
	}
	if len(options) > 0 && s.Same == nil {
		panic("headless: Select requires Same before SetOptions")
	}
	s.list.SetItems(options)
	for i := range s.list.items {
		s.list.items[i].Label = strings.Clone(s.list.items[i].Label)
	}
	if s.value != nil {
		if hadPrevious {
			s.sync()
			s.store()
		}
		return
	}
	if hadPrevious {
		for i, option := range s.list.items {
			if option.holds(previous.Value, s.Same) {
				s.list.Select(i)
				s.store()
				return
			}
		}
	}
	s.store()
}

// Options returns a copy of what is on offer.
func (s *Select[T]) Options() []Option[T] { return slices.Clone(s.list.items) }

// Chosen projects the current choice without changing the cursor or writing Value.
func (s *Select[T]) Chosen() (Option[T], bool) {
	at := s.list.Selected()
	if s.value != nil {
		at, _ = s.indexOf(s.value.Value())
	}
	return s.list.At(at)
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
	selected := s.list.Selected()
	if s.value != nil {
		selected, _ = s.indexOf(s.value.Value())
	}
	s.list.drawRows(s.frame(v, s.Label, look), selected, func(v grid.View, _ int, option Option[T], under bool) {
		if s.Row != nil {
			s.Row(v, option, under, under, look)
		} else {
			look.choice(v, option.Label, under, under)
		}
	})
}

func (s *Select[T]) indexOf(want T) (int, bool) {
	for i, option := range s.list.items {
		if option.holds(want, s.Same) {
			return i, true
		}
	}
	return s.list.Selected(), false
}

// Handle moves the cursor, and takes the choice with it.
func (s *Select[T]) Handle(ev input.Event) bool {
	s.Sync()
	if key, ok := ev.(input.Key); ok {
		_, handled := s.matcher.Handle(s.list.keys(), key, s.Do)
		return handled
	}
	if mouse, ok := ev.(input.Mouse); ok {
		ev = s.local(mouse)
	}
	before := s.list.Selected()
	handled := s.list.Handle(ev)
	if s.list.Selected() != before {
		s.store()
	}
	return handled
}

// Do runs one of the field's actions by name. See [Doer].
func (s *Select[T]) Do(action keymap.Action) bool {
	s.Sync()
	before := s.list.Selected()
	if !s.list.Do(action) {
		return false
	}
	if s.list.Selected() != before {
		s.store()
	}
	return true
}

// Validate checks the choice.
func (s *Select[T]) Validate() error {
	return s.validate(func() error {
		s.Sync()
		// An unmatched initial value is settled at this semantic boundary, once.
		s.store()
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
			_, matched := s.indexOf(s.value.Value())
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
		s.focus(has, s.list.Focus, s.Sync, s.Validate)
	}
	if has {
		change()
	} else {
		s.matcher.Reset(change)
	}
}

// Sync reconciles the cursor with Value without accepting an unavailable value.
func (s *Select[T]) Sync() {
	s.list.Keys = s.Keys
	s.sync()
}

// sync reconciles the cursor with the caller-owned value without accepting an
// unavailable value. Validation or an actual selection transition performs that
// settlement through store; a read alone is not an assignment.
func (s *Select[T]) sync() {
	if s.value == nil {
		return
	}
	s.syncValue(s.value.Value())
}

func (s *Select[T]) syncValue(value T) {
	// The cursor starts on the choice already made, which is what makes a form somebody
	// is coming back to show what they said last time.
	at, _ := s.indexOf(value)
	s.list.Select(at)
}

func (s *Select[T]) store() {
	if s.value == nil {
		return
	}
	if chosen, ok := s.list.Current(); ok {
		if chosen.holds(s.value.Value(), s.Same) {
			return
		}
		s.value.Set(chosen.Value)
		// Set may hand ownership to another accessor or to a newer local answer.
		s.sync()
	}
}
