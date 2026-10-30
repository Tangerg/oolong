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
// The zero value is empty and ready. A MultiSelect must not be copied after first use:
// its options, chosen set, cursor and matcher are one mutable field.
// Choices are stored once in option order. SetTaken canonicalizes incoming values
// under the configured equality and limit.
type MultiSelect[T any] struct {
	noCopy noCopy

	field
	Label string
	// Same says whether two values are the same one — see [Select.Same]. It is a pure
	// projection callback and may run during drawing.
	Same func(a, b T) bool
	// Check says what is wrong with the choices, or nil.
	Check func(v []T) error
	// Rows caps how many options are shown at once. Zero shows them all.
	Rows int
	// Keys say which keystrokes move and take. Nil reads through
	// [DefaultMultiSelectKeys].
	Keys *keymap.Map

	// Row optionally draws a one-row choice with its cursor and taken states.
	// Nil uses Look's default choice row. Like Draw, the callback must be pure.
	Row func(grid.View, Option[T], bool, bool, Look)

	list List[Option[T]]
	// taken is the sole chosen-set representation, in option order.
	taken    []bool
	limit    int
	matcher  keymap.Matcher
	revision uint64
}

// Prompt is what the field is asking for.
func (m *MultiSelect[T]) Prompt() string { return m.Label }

// SetOptions replaces what is on offer. MultiSelect owns the slice and preserves
// each taken choice that remains available under Same, wherever it moved.
// Non-empty options require Same; omitting it is a programmer error and panics.
// Required reads and identity comparisons finish before options or selection change.
// An abnormal exit during preparation preserves the previous field.
func (m *MultiSelect[T]) SetOptions(options []Option[T]) {
	requireMutable(m.element)
	if len(options) > 0 && m.Same == nil {
		panic("base: MultiSelect requires Same before SetOptions")
	}
	taken := make([]bool, len(options))
	for _, want := range m.takenOptions() {
		for i, option := range options {
			if !taken[i] && option.holds(want.Value, m.Same) {
				taken[i] = true
				break
			}
		}
	}
	clampTaken(taken, m.limit)
	m.matcher.Reset(func() {
		m.list.SetItems(options)
		for i := range m.list.items {
			m.list.items[i].Label = strings.Clone(m.list.items[i].Label)
		}
		m.taken = taken
		m.revision++
		m.element.Invalidate()
		m.invalidateValidation()
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
	m.limit = limit
	// Settle against the new limit directly, without publishing an intermediate set.
	if clampTaken(m.taken, m.limit) {
		m.revision++
		m.element.Invalidate()
	}
}

// Limit reports how many choices may be taken at once. Zero allows every option.
func (m *MultiSelect[T]) Limit() int {
	return m.limit
}

// Taken returns a copy of the selected values in option order.
func (m *MultiSelect[T]) Taken() []T {
	taken, _ := m.selection()
	return m.valuesOf(taken)
}

func (m *MultiSelect[T]) valuesOf(taken []bool) []T {
	var out []T
	for i, option := range m.list.items {
		if i < len(taken) && taken[i] {
			out = append(out, option.Value)
		}
	}
	return out
}

func (m *MultiSelect[T]) takenOptions() []Option[T] {
	var out []Option[T]
	for i, option := range m.list.items {
		if i < len(m.taken) && m.taken[i] {
			out = append(out, option)
		}
	}
	return out
}

func takenCount(taken []bool) int {
	count := 0
	for _, selected := range taken {
		if selected {
			count++
		}
	}
	return count
}

// clampTaken is the one enforcement point for a choice limit. It preserves option
// order, which is the order MultiSelect reports and the only stable priority
// available.
func clampTaken(taken []bool, limit int) bool {
	if limit <= 0 {
		return false
	}
	kept := 0
	changed := false
	for i, selected := range taken {
		if !selected {
			continue
		}
		if kept < limit {
			kept++
			continue
		}
		taken[i] = false
		changed = true
	}
	return changed
}

// Toggle takes the option under the cursor, or gives it back, and reports whether
// anything changed. Nothing changes when the limit is reached.
func (m *MultiSelect[T]) Toggle() bool {
	requireMutable(m.element)
	return m.toggle()
}

func (m *MultiSelect[T]) toggle() bool {
	before, _ := m.selection()
	at := m.list.Selected()
	if at < 0 || at >= len(before) {
		return false
	}
	if !before[at] && m.limit > 0 && takenCount(before) >= m.limit {
		return false
	}
	proposed := slices.Clone(before)
	proposed[at] = !proposed[at]
	m.store(proposed)
	after, _ := m.selection()
	return !slices.Equal(before, after)
}

// HeightForWidth is the label, the options within their cap, and the problem if there is one.
func (m *MultiSelect[T]) HeightForWidth(int) int {
	rows := len(m.list.items)
	if m.Rows > 0 {
		rows = min(rows, m.Rows)
	}
	return layout.Sum(max(rows, 1), m.rows(m.Label))
}

// Draw paints the label, the options and whatever was wrong with the choices.
func (m *MultiSelect[T]) Draw(v scene.Frame) {
	m.DrawWith(v, Look{})
}

// DrawWith paints the field using this frame's look without changing configuration.
func (m *MultiSelect[T]) DrawWith(v scene.Frame, look Look) {
	taken, _ := m.selection()
	m.list.DrawRows(m.frame(v, m.Label, look), func(v grid.View, at int, option Option[T], under bool) {
		chosen := at < len(taken) && taken[at]
		if m.Row != nil {
			m.Row(v, option, under, chosen, look)
		} else {
			look.choice(v, option.Label, under, chosen)
		}
	})
}

func (m *MultiSelect[T]) selection() ([]bool, bool) { return slices.Clone(m.taken), true }

func (m *MultiSelect[T]) selectionOf(options []Option[T], bound []T) ([]bool, bool) {
	taken := make([]bool, len(options))
	for _, want := range bound {
		for i, option := range options {
			if option.holds(want, m.Same) {
				taken[i] = true
				break
			}
		}
	}
	clamped := clampTaken(taken, m.limit)
	count := 0
	for _, selected := range taken {
		if selected {
			count++
		}
	}
	if clamped || len(bound) != count {
		return taken, false
	}
	at := 0
	for i, option := range options {
		if !taken[i] {
			continue
		}
		if !option.holds(bound[at], m.Same) {
			return taken, false
		}
		at++
	}
	return taken, true
}

// Handle moves the cursor and takes choices.
func (m *MultiSelect[T]) Handle(ev input.Event) bool {
	requireMutable(m.element)
	if key, ok := ev.(input.Key); ok {
		_, handled := m.matcher.Handle(m.keys(), key, m.Do)
		return handled
	}
	mouse, ok := ev.(input.Mouse)
	if !ok {
		return false
	}
	return m.list.Handle(m.local(mouse))
}

// Do runs one of the field's actions by name. See [Doer].
func (m *MultiSelect[T]) Do(action keymap.Action) bool {
	requireMutable(m.element)
	if action == Toggle {
		m.toggle()
		return true
	}
	return m.list.Do(action)
}

// Validate checks the choices.
func (m *MultiSelect[T]) Validate() error { return m.validate(nil, m.checkAnswer) }

func (m *MultiSelect[T]) checkAnswer() fieldCheck {
	answer := m.Taken()
	revision := m.Revision()
	checked := fieldCheck{current: func() bool { return m.Revision() == revision }}
	if m.Check != nil {
		checked.err = m.Check(slices.Clone(answer))
	}
	return checked
}

// Focus takes the keyboard or gives it up, and checks the choices on the way out.
func (m *MultiSelect[T]) focusChanged(has bool) {
	change := func() {
		m.focus(has, m.list.focusChanged, nil, m.Validate)
	}
	if has {
		change()
	} else {
		m.matcher.Reset(change)
	}
}

// SetTaken replaces the chosen set. Unavailable and duplicate values are discarded.
func (m *MultiSelect[T]) SetTaken(values []T) {
	taken, _ := m.selectionOf(m.list.items, values)
	m.store(taken)
}

func (m *MultiSelect[T]) store(taken []bool) {
	requireMutable(m.element)
	if slices.Equal(m.taken, taken) {
		return
	}
	m.taken = taken
	m.revision++
	m.element.Invalidate()
}

func (m *MultiSelect[T]) keys() *keymap.Map {
	if m.Keys != nil {
		return m.Keys
	}
	return multiSelectKeys()
}

// Revision advances only when the canonical chosen set changes.
func (m *MultiSelect[T]) Revision() uint64 { return m.revision }
