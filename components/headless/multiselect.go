package headless

import (
	"slices"
	"strings"

	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/keymap"
	"github.com/Tangerg/oolong/core/layout"
)

// MultiSelect is a field holding any number of choices out of several.
//
// The cursor and the choice are two things here, unlike in a [Select]: moving is not
// choosing, and something has to be pressed. That is the whole difference between
// picking one and picking some.
//
// The zero value is empty and ready. A MultiSelect must not be copied after first use:
// its options, chosen set, cursor and matcher are one mutable field.
// NewMultiSelect configures a caller-owned set; SetBinding transfers ownership.
// Reads project available choices in option order. Sync offers the canonical set,
// discarding unavailable and duplicate values while respecting the configured limit.
type MultiSelect[T any] struct {
	noCopy noCopy

	field
	value Accessor[[]T]
	// owner identifies the binding lifetime independently of command progress.
	owner *byte
	// Label is what the field is asking for.
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
	// taken owns local answers only. Controlled answers are projected from value;
	// operations offer an independent proposal without keeping a writable mirror.
	taken   []bool
	limit   int
	matcher keymap.Matcher
	// operation revokes a command when normalization starts a newer semantic call.
	operation uint64
	settling  *byte
}

// SetBinding transfers the answer to a new accessor and invalidates old validation
// and key sequences. Nil retains the currently accepted answer as local state.
// The completed transfer is visible to resolver cancellation callbacks.
func (m *MultiSelect[T]) SetBinding(value Accessor[[]T]) {
	var accepted []bool
	if value == nil {
		accepted, _ = m.selection()
	}
	m.matcher.Reset(func() {
		m.operation++
		m.value, m.owner = value, new(byte)
		m.taken = accepted
		m.invalidateValidation()
	})
}

// Prompt is what the field is asking for.
func (m *MultiSelect[T]) Prompt() string { return m.Label }

// SetOptions replaces what is on offer. MultiSelect owns the slice and preserves
// each taken choice that remains available under Same, wherever it moved.
// Non-empty options require Same; omitting it is a programmer error and panics.
func (m *MultiSelect[T]) SetOptions(options []Option[T]) {
	m.operation++
	m.matcher.Reset(func() { m.setOptions(options) })
}

func (m *MultiSelect[T]) setOptions(options []Option[T]) {
	hadOptions := m.list.Len() > 0
	var previous []Option[T]
	if m.value == nil {
		previous = m.takenOptions()
	}
	if len(options) > 0 && m.Same == nil {
		panic("headless: MultiSelect requires Same before SetOptions")
	}
	m.list.SetItems(options)
	for i := range m.list.items {
		m.list.items[i].Label = strings.Clone(m.list.items[i].Label)
	}
	if m.value != nil {
		if hadOptions {
			m.Sync()
		}
		return
	}
	m.taken = make([]bool, len(m.list.items))
	for _, want := range previous {
		for i, option := range m.list.items {
			if !m.taken[i] && option.holds(want.Value, m.Same) {
				m.taken[i] = true
				break
			}
		}
	}
	clampTaken(m.taken, m.limit)
}

// Options returns a copy of what is on offer.
func (m *MultiSelect[T]) Options() []Option[T] { return slices.Clone(m.list.items) }

// SetLimit changes how many choices may be taken at once. Zero allows every option;
// a negative limit is a programmer error and panics, because zero already means "no
// limit" and there is no smaller quantity of choices for a negative one to name.
// Lowering the limit keeps the earliest choices in option order and writes the settled
// set back to a bound value.
func (m *MultiSelect[T]) SetLimit(limit int) {
	if limit < 0 {
		panic("headless: multi-select limit cannot be negative")
	}
	m.limit = limit
	m.operation++
	// Settle against the new limit directly, without publishing an intermediate set.
	m.Sync()
}

// Limit reports how many choices may be taken at once. Zero allows every option.
func (m *MultiSelect[T]) Limit() int {
	return m.limit
}

// Taken projects the available choices in option order without writing Value.
// Use Sync to explicitly settle duplicates, unavailable choices and the limit.
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
	if !m.beginOperation() {
		return false
	}
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
func (m *MultiSelect[T]) Draw(v Frame) {
	m.DrawWith(v, Look{})
}

// DrawWith paints the field using this frame's look without changing configuration.
func (m *MultiSelect[T]) DrawWith(v Frame, look Look) {
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

func (m *MultiSelect[T]) selection() ([]bool, bool) {
	binding := m.value
	if binding == nil {
		taken := make([]bool, len(m.list.items))
		copy(taken, m.taken)
		return taken, !clampTaken(taken, m.limit)
	}
	return m.selectionOf(binding.Value())
}

func (m *MultiSelect[T]) selectionOf(bound []T) ([]bool, bool) {
	taken := make([]bool, len(m.list.items))
	for _, want := range bound {
		for i, option := range m.list.items {
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
	for i, option := range m.list.items {
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
	if !m.beginOperation() {
		return true
	}
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
	if !m.beginOperation() {
		return false
	}
	if action == Toggle {
		m.toggle()
		return true
	}
	return m.list.Do(action)
}

// Validate checks the choices.
func (m *MultiSelect[T]) Validate() error {
	return m.validate(func() error {
		if !m.beginOperation() {
			return ErrValidationSuperseded
		}
		return nil
	}, m.checkAnswer)
}

func (m *MultiSelect[T]) checkAnswer() fieldCheck {
	answer := m.Taken()
	checked := fieldCheck{current: func() bool {
		taken, canonical := m.selection()
		return canonical && slices.EqualFunc(m.valuesOf(taken), answer, m.Same)
	}}
	if !checked.current() {
		return fieldCheck{err: ErrInvalidBinding}
	}
	if m.Check != nil {
		checked.err = m.Check(slices.Clone(answer))
	}
	return checked
}

// Focus takes the keyboard or gives it up, and checks the choices on the way out.
func (m *MultiSelect[T]) Focus(has bool) {
	change := func() {
		m.focus(has, m.list.Focus, func() { m.beginOperation() }, m.Validate)
	}
	if has {
		change()
	} else {
		m.matcher.Reset(change)
	}
}

// Sync makes the caller-owned set the selection model's source. Unlike Draw, this is
// a semantic boundary, so it also settles a non-canonical value exactly once.
//
// It hands the list no key map. The list inside has none of its
// own: this field resolves every keystroke against one that has the movement and the
// key that takes a choice in it, and drives the list by name. Offering the event to
// both would resolve it twice.
// Sync reports whether it remained current through normalization. A newer semantic
// operation started by the accessor revokes it; input callers must then stop dispatch.
func (m *MultiSelect[T]) Sync() bool {
	operation := m.operation
	taken, canonical := m.selection()
	if !canonical && m.value != nil && m.owner != m.settling {
		m.operation++
		operation = m.operation
		m.store(taken)
	} else if m.value == nil {
		m.taken = taken
	}
	return operation == m.operation
}

func (m *MultiSelect[T]) beginOperation() bool {
	m.operation++
	return m.Sync()
}

func (m *MultiSelect[T]) store(taken []bool) {
	if binding := m.value; binding != nil {
		previous := m.settling
		m.settling = m.owner
		defer func() { m.settling = previous }()
		binding.Set(m.valuesOf(taken))
		return
	}
	m.taken = taken
}

func (m *MultiSelect[T]) keys() *keymap.Map {
	if m.Keys != nil {
		return m.Keys
	}
	return multiSelectKeys()
}
