package base

import (
	"slices"
	"strings"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/keymap"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/scene"
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

// MultiSelectConfig supplies available options and an initial chosen set. Unknown values and duplicates are removed at the owner boundary.
type MultiSelectConfig[T any] struct {
	Key     string
	Initial []T
	Options []Option[T]
	Label   string
	// Same has the same lifetime and purity contract as SelectConfig.Same.
	Same  func(T, T) bool
	Check func([]T) error
	Rows  int
	// Limit is how many choices may be taken at once; zero allows every option.
	// It applies before Initial, which must respect it.
	Limit int
	Keys  *keymap.Map
	Row   func(grid.View, Option[T], bool, bool, Look)
}

// NewMultiSelect panics for negative Rows or Limit, an Initial set larger than
// Limit, or non-empty Options without Same. Its chosen set is canonical; returned
// slices are detached observations.
func NewMultiSelect[T any](config MultiSelectConfig[T]) *MultiSelect[T] {
	requireNonNegative("choice Rows", config.Rows)
	requireNonNegative("multi-select Limit", config.Limit)
	m := &MultiSelect[T]{check: config.Check, limit: config.Limit}
	m.initialize(config.Label, config.Same, config.Rows, config.Keys, config.Row)
	m.key = strings.Clone(config.Key)
	m.SetOptions(config.Options)
	if taken, _ := m.indices(m.list.items, config.Initial); !m.admits(len(taken)) {
		panic("base: multi-select Initial exceeds its Limit")
	}
	m.SetTaken(config.Initial)
	return m
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
	truncate := !limitAdmits(limit, len(m.taken))
	if truncate {
		_ = nextRevision(m.revision)
	}
	m.limit = limit
	m.invalidateValidation()
	if truncate {
		m.store(m.taken[:limit])
		return
	}
	m.element.Invalidate()
}

// limitAdmits is the one choice-limit rule. Interactive paths refuse a count it
// does not admit; programmatic replacement keeps the earliest choices it does.
func limitAdmits(limit, count int) bool { return limit == 0 || count <= limit }

func (m *MultiSelect[T]) admits(count int) bool { return limitAdmits(m.limit, count) }

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

// KeysFor is what the effective key map binds to action, defaults included.
func (m *MultiSelect[T]) KeysFor(action keymap.Action) []input.Keys {
	return m.keys.bound(multiSelectKeys, action)
}

func (m *MultiSelect[T]) Element() scene.Element {
	if m.element.IsZero() {
		m.adopt(scene.NewElement(scene.ElementConfig{
			Key: m.key, Measure: layout.MeasureRows(m.HeightForWidth), Paint: m.Draw,
			Handle: func(e scene.Event) scene.EventResult {
				return partEvent(e, m.Handle, &m.geometry, &m.list.geometry, &m.list.pointer.geometry)
			}, Focusable: func() bool { return true }, FocusChanged: m.focusChanged,
			Semantics: func() scene.Semantic {
				return withFeedback(scene.Semantic{Role: scene.RoleList, Name: m.label, Value: m.takenLabels(), ContentRevision: m.list.contentRevision}, m.Error())
			}, Actions: append(collectionActions(&m.list, m.choiceState), m.setTakenAction()),
		}))
	}
	return m.element
}
