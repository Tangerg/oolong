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

	field
	label string
	// Rows caps how many options are shown at once. Zero shows them all.
	rowLimit int
	// Keys say which keystrokes move and take. Nil reads through
	// [DefaultMultiSelectKeys].
	keys keyBinding

	// Row optionally draws a one-row choice with its cursor and taken states.
	// Nil uses Look's default choice row. Like Draw, the callback must be pure.
	rowPainter func(grid.View, Option[T], bool, bool, Look)

	list List[Option[T]]
	// Each selected value is represented by its first equivalent option index.
	taken        []int
	limit        int
	matcher      keymap.Matcher
	sameIdentity func(T, T) bool
	check        func([]T) error
	revision     uint64
}

func (m *MultiSelect[T]) Prompt() string { return m.label }

// SetOptions replaces what is on offer. MultiSelect owns the slice and preserves
// each taken identity that remains available, wherever it moved. The replacement
// supplies the complete values, so a non-empty prior answer advances Revision even
// when every identity is retained. A zero-value MultiSelect panics on non-empty
// options; construct one with Same instead.
// Required reads and identity comparisons finish before options or selection change.
// An abnormal exit during preparation preserves the previous field.
func (m *MultiSelect[T]) SetOptions(options []Option[T]) {
	requireMutable(m.element)
	if len(options) > 0 && m.sameIdentity == nil {
		panic("base: MultiSelect requires Same in its construction config")
	}
	before := m.Taken()
	taken, _ := m.indices(options, before)
	m.matcher.Reset(func() {
		m.list.replaceItems(options, m.list.selected)
		for i := range m.list.items {
			m.list.items[i].Label = strings.Clone(m.list.items[i].Label)
		}
		m.taken = taken
		if len(before) != 0 {
			m.revision++
		}
		m.invalidateValidation()
		m.list.notifyChange()
	})
}

// Options returns a copy of what is on offer.
func (m *MultiSelect[T]) Options() []Option[T] { return slices.Clone(m.list.items) }

// SetLimit changes how many choices may be taken at once. Zero allows every option;
// a negative limit is a programmer error and panics, because zero already means "no
// limit" and there is no smaller quantity of choices for a negative one to name.
// Lowering the limit keeps the earliest choices in option order.
func (m *MultiSelect[T]) SetLimit(limit int) {
	requireMutable(m.element)
	if limit < 0 {
		panic("base: multi-select limit cannot be negative")
	}
	if m.limit == limit {
		return
	}
	m.limit = limit
	m.invalidateValidation()
	if limit > 0 && len(m.taken) > limit {
		m.store(m.taken[:limit])
		return
	}
	m.element.Invalidate()
}

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
		if m.limit > 0 && len(m.taken) >= m.limit {
			return false
		}
		m.store(slices.Insert(slices.Clone(m.taken), pos, at))
	}
	return true
}

func (m *MultiSelect[T]) HeightForWidth(int) int {
	rows := len(m.list.items)
	if m.rowLimit > 0 {
		rows = min(rows, m.rowLimit)
	}
	return layout.Sum(max(rows, 1), m.rows(m.label))
}

func (m *MultiSelect[T]) Draw(v scene.Frame) {
	m.DrawWith(v, Look{})
}

func (m *MultiSelect[T]) DrawWith(v scene.Frame, look Look) {
	m.list.DrawRows(m.frame(v, m.label, look), func(v grid.View, _ int, option Option[T], under bool) {
		chosen := readProjection(m.element, func() bool {
			return slices.ContainsFunc(m.taken, func(at int) bool {
				return m.sameIdentity(m.list.items[at].Value, option.Value)
			})
		})
		if m.rowPainter != nil {
			m.rowPainter(v, option, under, chosen, look)
		} else {
			look.choice(v, option.Label, under, chosen)
		}
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

func (m *MultiSelect[T]) focusChanged(has bool) {
	change := func() {
		m.focus(has, m.list.focusChanged, m.Validate)
	}
	if has {
		change()
	} else {
		m.matcher.Reset(change)
	}
}

// SetTaken replaces the chosen set. Unavailable and duplicate values are discarded.
func (m *MultiSelect[T]) SetTaken(values []T) {
	requireMutable(m.element)
	taken, _ := m.indices(m.list.items, values)
	if m.limit > 0 && len(taken) > m.limit {
		taken = taken[:m.limit]
	}
	m.store(taken)
}

func (m *MultiSelect[T]) store(taken []int) {
	requireMutable(m.element)
	if slices.Equal(m.taken, taken) {
		return
	}
	m.taken = taken
	m.revision++
	m.element.Invalidate()
}

func (m *MultiSelect[T]) resolvedKeys() *keymap.Map {
	if m.keys.current != nil {
		return m.keys.current
	}
	return multiSelectKeys()
}

// Revision identifies the canonical chosen values. Replacing their option
// collection advances it even when the identities match; cursor movement alone
// does not replace an answer.
func (m *MultiSelect[T]) Revision() uint64 { return m.revision }

func (m *MultiSelect[T]) Label() string { return m.label }

func (m *MultiSelect[T]) SetLabel(value string) {
	element := m.Element()
	requireMutable(element)
	if m.label == value {
		return
	}
	m.label = strings.Clone(value)
	element.Invalidate()
}

func (m *MultiSelect[T]) Rows() int { return m.rowLimit }

// SetRows panics for a negative limit. Zero permits all options to contribute to height.
func (m *MultiSelect[T]) SetRows(value int) {
	element := m.Element()
	requireMutable(element)
	if value < 0 {
		panic("base: Rows cannot be negative")
	}
	if m.rowLimit == value {
		return
	}
	m.rowLimit = value
	element.Invalidate()
}

func (m *MultiSelect[T]) Row() func(grid.View, Option[T], bool, bool, Look) { return m.rowPainter }

func (m *MultiSelect[T]) SetRow(value func(grid.View, Option[T], bool, bool, Look)) {
	element := m.Element()
	requireMutable(element)
	m.rowPainter = value
	element.Invalidate()
}

func (m *MultiSelect[T]) Keys() *keymap.Map { return m.keys.current }

func (m *MultiSelect[T]) SetKeys(keys *keymap.Map) {
	m.keys.set(m.Element(), &m.matcher, keys)
}
