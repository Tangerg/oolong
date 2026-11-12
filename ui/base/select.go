package base

import (
	"slices"
	"strings"

	"github.com/Tangerg/oolong/ui/core/scene"

	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/keymap"
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

	choiceList[T]
	check func(T) error
}

// SetOptions replaces what is on offer. Select owns the slice. If the selected
// choice still exists under Same, the cursor follows it to its new position.
// Otherwise the cursor is clamped. A zero-value Select panics on non-empty
// options; construct one with Same instead.
// Replacing an available answer advances Revision even when its identity is retained:
// identity does not establish that the complete payload is unchanged.
// Required reads and identity comparisons finish before options or selection change.
// An abnormal exit during preparation preserves the previous field.
// Exhausting the answer or collection revision panics before either changes.
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
	revision := s.revision
	if replacesAnswer {
		revision = nextRevision(revision)
	}
	s.list.requireReplacement()
	s.matcher.Reset(func() {
		s.list.replaceItems(options, at)
		for i := range s.list.items {
			s.list.items[i].Label = strings.Clone(s.list.items[i].Label)
		}
		s.revision = revision
		s.invalidateValidation()
		s.list.notifyChange()
	})
}

func (s *Select[T]) Chosen() (Option[T], bool) {
	return s.list.At(s.selected())
}

func (s *Select[T]) selected() int { return s.list.Selected() }

func (s *Select[T]) Draw(v scene.Frame) {
	s.DrawWith(v, Look{})
}

func (s *Select[T]) DrawWith(v scene.Frame, look Look) {
	s.drawRows(v, look, func(_ Option[T], under bool) bool { return under })
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
		_, handled := s.matcher.Handle(s.keys.resolve(listKeys), key, keyboardReceipt(&s.element), s.Do)
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

func (s *Select[T]) focusChanged(has bool) { s.choiceList.focusChanged(has, s.Validate) }

// Choose selects an available value using the configured identity rule.
// Exhausting the answer revision panics before changing selection.
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
		s.revision = nextRevision(s.revision)
	}
	s.list.Select(at)
}
