package base

import (
	"slices"
	"strings"

	"github.com/Tangerg/oolong/ui/core/scene"

	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/keymap"
)

// MultiSelect is a field holding any number of choices out of several.
//
// The cursor and the choice are two things here, unlike in a [Select]: moving is not
// choosing, and something has to be pressed. That is the whole difference between
// picking one and picking some.
//
// The zero value is empty; non-empty options require NewMultiSelect with Same.
// A MultiSelect must not be copied after first use: its options, chosen set, cursor
// and matcher are one mutable field. Options with the same identity are aliases of
// one choice. Its first option supplies the value and position in Taken, and
// selecting any alias applies to all of them.
type MultiSelect[T any] struct {
	noCopy noCopy

	choiceList[T]
	// Each selected value is represented by its first equivalent option index.
	taken []int
	limit int
	check func([]T) error
}

// SetOptions replaces what is on offer. MultiSelect owns the slice and preserves
// each taken identity that remains available, wherever it moved. The replacement
// supplies the complete values, so a non-empty prior answer advances Revision even
// when every identity is retained. A zero-value MultiSelect panics on non-empty
// options; construct one with Same instead.
// Required reads and identity comparisons finish before options or selection change.
// An abnormal exit during preparation preserves the previous field.
// Exhausting the answer or collection revision panics before either changes.
func (m *MultiSelect[T]) SetOptions(options []Option[T]) {
	requireMutable(m.element)
	if len(options) > 0 && m.sameIdentity == nil {
		panic("base: MultiSelect requires Same in its construction config")
	}
	before := m.Taken()
	taken, _ := m.indices(options, before)
	revision := m.revision
	if len(before) != 0 {
		revision = nextRevision(revision)
	}
	m.list.requireReplacement()
	m.matcher.Reset(func() {
		m.list.replaceItems(options, m.list.selected)
		for i := range m.list.items {
			m.list.items[i].Label = strings.Clone(m.list.items[i].Label)
		}
		m.taken = taken
		m.revision = revision
		m.invalidateValidation()
		m.list.notifyChange()
	})
}

// SetLimit changes how many choices may be taken at once. Zero allows every option;
// a negative limit is a programmer error and panics, because zero already means "no
// limit" and there is no smaller quantity of choices for a negative one to name.
// Lowering the limit keeps the earliest choices in option order.
// Exhausting the answer revision panics before changing the limit or answer.
func (m *MultiSelect[T]) SetLimit(limit int) {
	requireMutable(m.element)
	requireNonNegative("multi-select Limit", limit)
	if m.limit == limit {
		return
	}
	if limit != 0 && len(m.taken) > limit {
		_ = nextRevision(m.revision)
	}
	m.limit = limit
	m.invalidateValidation()
	if !m.admits(len(m.taken)) {
		m.store(m.taken[:limit])
		return
	}
	m.element.Invalidate()
}

// admits is the one choice-limit rule. Interactive paths refuse a count it does
// not admit; programmatic replacement keeps the earliest choices it does.
func (m *MultiSelect[T]) admits(count int) bool { return m.limit == 0 || count <= m.limit }

// Limit reports how many choices may be taken at once. Zero allows every option.
func (m *MultiSelect[T]) Limit() int {
	return m.limit
}

// Taken returns a copy of the selected values in option order.
func (m *MultiSelect[T]) Taken() []T {
	if len(m.taken) == 0 {
		return nil
	}
	out := make([]T, len(m.taken))
	for i, at := range m.taken {
		out[i] = m.list.items[at].Value
	}
	return out
}

func (m *MultiSelect[T]) indices(options []Option[T], values []T) ([]int, bool) {
	var indices []int
	complete := true
	m.Element().ReadOnly(func() {
		for _, value := range values {
			if at := optionIndex(options, value, m.sameIdentity); at >= 0 {
				indices = append(indices, at)
			} else {
				complete = false
			}
		}
	})
	slices.Sort(indices)
	return slices.Compact(indices), complete
}

// Toggle takes the option under the cursor, or gives it back, and reports whether
// anything changed. Nothing changes when the limit is reached.
// Exhausting the answer revision panics before changing it.
func (m *MultiSelect[T]) Toggle() bool {
	requireMutable(m.element)
	return m.toggle()
}

func (m *MultiSelect[T]) toggle() bool {
	option, ok := m.list.Current()
	if !ok {
		return false
	}
	at := readProjection(m.Element(), func() int { return optionIndex(m.list.items, option.Value, m.sameIdentity) })
	if at < 0 {
		return false
	}
	pos, selected := slices.BinarySearch(m.taken, at)
	if selected {
		m.store(slices.Delete(slices.Clone(m.taken), pos, pos+1))
	} else {
		if !m.admits(len(m.taken) + 1) {
			return false
		}
		m.store(slices.Insert(slices.Clone(m.taken), pos, at))
	}
	return true
}

func (m *MultiSelect[T]) Draw(v scene.Frame) {
	m.DrawWith(v, Look{})
}

func (m *MultiSelect[T]) DrawWith(v scene.Frame, look Look) {
	m.drawRows(v, look, func(option Option[T], _ bool) bool {
		return readProjection(m.element, func() bool {
			return slices.ContainsFunc(m.taken, func(at int) bool {
				return m.sameIdentity(m.list.items[at].Value, option.Value)
			})
		})
	})
}

func (m *MultiSelect[T]) Handle(ev input.Event) bool {
	requireMutable(m.element)
	if key, ok := ev.(input.Key); ok {
		_, handled := m.matcher.Handle(m.resolvedKeys(), key, keyboardReceipt(&m.element), m.Do)
		return handled
	}
	mouse, ok := ev.(input.Mouse)
	if !ok {
		return false
	}
	return m.list.Handle(m.local(mouse))
}

func (m *MultiSelect[T]) Do(action keymap.Action) bool {
	requireMutable(m.element)
	if action == Toggle {
		m.toggle()
		return true
	}
	return m.list.Do(action)
}

func (m *MultiSelect[T]) Validate() error { return m.validate(nil, m.Revision, m.checkAnswer) }

// SetCheck revokes old results because validation belongs to the rule as well as
// the answer. Reinstalling the same callback also revokes receipts; nil removes it.
func (m *MultiSelect[T]) SetCheck(check func([]T) error) {
	requireMutable(m.element)
	m.check = check
	m.InvalidateValidation()
}

func (m *MultiSelect[T]) checkAnswer() error {
	if m.check == nil {
		return nil
	}
	return m.check(m.Taken())
}

func (m *MultiSelect[T]) focusChanged(has bool) { m.choiceList.focusChanged(has, m.Validate) }

// SetTaken replaces the chosen set. Unavailable and duplicate values are discarded,
// and beyond the limit the earliest choices in option order are kept, as when the
// limit is lowered.
// Exhausting the answer revision panics before changing it.
func (m *MultiSelect[T]) SetTaken(values []T) {
	requireMutable(m.element)
	taken, _ := m.indices(m.list.items, values)
	if !m.admits(len(taken)) {
		taken = taken[:m.limit]
	}
	m.store(taken)
}

func (m *MultiSelect[T]) store(taken []int) {
	requireMutable(m.element)
	if slices.Equal(m.taken, taken) {
		return
	}
	revision := nextRevision(m.revision)
	m.taken, m.revision = taken, revision
	m.element.Invalidate()
}

func (m *MultiSelect[T]) resolvedKeys() *keymap.Map { return m.keys.resolve(multiSelectKeys) }
