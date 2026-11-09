package base

import (
	"context"
	"slices"

	"github.com/Tangerg/oolong/ui/core/scene"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/keymap"
)

// Table owns rows and their sort order. Its private List prevents replacement
// from bypassing sorting; appearance owns column geometry. The zero value is ready.
// A Table must not be copied after first use. Comparisons must be pure; reentrant
// ordering mutations panic and failed comparisons preserve the previous state.
type Table[T any] struct {
	element scene.Element
	noCopy  noCopy

	rows List[T]

	sortColumns func(column int) func(a, b T) int

	order *SortOrder
}

type SortOrder struct {
	Column     int
	Descending bool
}

type TableConfig[T any] struct {
	Items []T
	Row   func(grid.View, int, T, bool)
	Keys  *keymap.Map
	Wrap  bool
}

func NewTable[T any](config TableConfig[T]) *Table[T] {
	t := &Table[T]{}
	t.rows.rowPainter, t.rows.keys.current, t.rows.wrap = config.Row, config.Keys, config.Wrap
	t.SetItems(config.Items)
	return t
}

const (
	ActionSort      scene.ActionID = "sort"
	ActionClearSort scene.ActionID = "clear-sort"
)

// SetSortColumns keeps availability and ordering with one column owner: nil means unsupported.
// Columns and their pure strict weak comparators stay stable until replaced here.
// A removed active column clears sort metadata without moving rows. Failed lookup
// or comparison preserves the previous columns, rows and reported order.
func (t *Table[T]) SetSortColumns(columns func(column int) func(a, b T) int) {
	requireMutable(t.element)
	if t.order == nil {
		t.sortColumns = columns
		t.element.Invalidate()
		return
	}
	compare := t.columnComparator(columns, t.order.Column)
	if compare == nil {
		t.sortColumns = columns
		t.ClearSort()
		return
	}
	items, selected := t.sortRows(t.rows.Items(), t.rows.Selected(), compare, *t.order)
	t.rows.matcher.Reset(func() {
		t.sortColumns = columns
		t.replaceRows(items, selected)
	})
}

// SortBy orders rows by column and reports whether the order changed. Rows and sort
// metadata publish together after comparison succeeds. Repeating the current column
// reverses the order.
func (t *Table[T]) SortBy(column int) bool {
	order := SortOrder{Column: column}
	if t.order != nil && t.order.Column == column {
		order.Descending = !t.order.Descending
	}
	return t.SetSort(order)
}

// SetSort keeps retries idempotent; interactive direction toggles belong to SortBy.
func (t *Table[T]) SetSort(order SortOrder) bool {
	requireMutable(t.element)
	if t.rows.Len() == 0 || t.order != nil && *t.order == order {
		return false
	}
	compare := t.columnComparator(t.sortColumns, order.Column)
	if compare == nil {
		return false
	}
	items, selected := t.sortRows(t.rows.Items(), t.rows.Selected(), compare, order)
	t.rows.matcher.Reset(func() {
		t.order = &order
		t.replaceRows(items, selected)
	})
	return true
}

func (t *Table[T]) canSort(column int) bool {
	return t.rows.Len() > 0 && t.columnComparator(t.sortColumns, column) != nil
}

func (t *Table[T]) columnComparator(columns func(int) func(T, T) int, column int) func(T, T) int {
	if columns == nil || column < 0 {
		return nil
	}
	return readProjection(t.Element(), func() func(T, T) int { return columns(column) })
}

// Sorted returns the sort column, reverse flag and whether sorting is active.
// An unsorted table returns (0, false, false).
func (t *Table[T]) Sorted() (column int, descending, ok bool) {
	if t.order == nil {
		return 0, false, false
	}
	return t.order.Column, t.order.Descending, true
}

// ClearSort clears sort metadata without moving rows.
func (t *Table[T]) ClearSort() {
	requireMutable(t.element)
	if t.order != nil {
		t.order = nil
		t.element.Invalidate()
	}
}

// SetItems reapplies the active sort before publishing rows. A lookup that revokes
// that column without SetSortColumns violates its stability contract and panics.
func (t *Table[T]) SetItems(items []T) {
	requireMutable(t.element)
	items = slices.Clone(items)
	selected := min(max(t.rows.selected, 0), max(len(items)-1, 0))
	if t.order != nil {
		compare := t.columnComparator(t.sortColumns, t.order.Column)
		if compare == nil {
			panic("base: sort columns changed without SetSortColumns")
		}
		items, selected = t.sortRows(items, selected, compare, *t.order)
	}
	t.rows.matcher.Reset(func() {
		t.replaceRows(items, selected)
	})
}

func (t *Table[T]) Items() []T { return t.rows.Items() }

func (t *Table[T]) Len() int { return t.rows.Len() }

func (t *Table[T]) At(index int) (T, bool) { return t.rows.At(index) }

// Selected is the index of the row under the cursor, or -1.
func (t *Table[T]) Selected() int { return t.rows.Selected() }

func (t *Table[T]) Current() (T, bool) { return t.rows.Current() }

func (t *Table[T]) Select(i int) { t.rows.Select(i) }

func (t *Table[T]) Move(n int) { t.rows.Move(n) }

func (t *Table[T]) Scroll() *Scroll { return t.rows.Scroll() }

func (t *Table[T]) focusChanged(has bool) { t.rows.focusChanged(has) }

func (t *Table[T]) Focused() bool { return t.rows.Focused() }

func (t *Table[T]) Handle(ev input.Event) bool { return t.rows.Handle(ev) }

func (t *Table[T]) Do(action keymap.Action) bool { return t.rows.Do(action) }

func (t *Table[T]) HeightForWidth(width int) int { return t.rows.HeightForWidth(width) }

func (t *Table[T]) Draw(v scene.Frame) { t.rows.Draw(v) }

// DrawRows paints the rows that fit with a caller's own row painter — see
// [List.DrawRows].
func (t *Table[T]) DrawRows(v scene.Frame, draw func(grid.View, int, T, bool)) {
	t.rows.DrawRows(v, draw)
}

func (t *Table[T]) replaceRows(items []T, selected int) {
	t.rows.replaceItems(items, selected)
	t.rows.notifyChange()
}

// Sorting a permutation preserves the selected row without requiring T equality.
// Preparation borrows only private rows; publication and resolver cancellation wait
// until the comparison has returned, so a failure cannot claim an unfinished order.
func (t *Table[T]) sortRows(items []T, selected int, compare func(T, T) int, order SortOrder) ([]T, int) {
	if len(items) < 2 {
		return items, selected
	}

	permutation := make([]int, len(items))
	for i := range permutation {
		permutation[i] = i
	}
	t.Element().ReadOnly(func() {
		slices.SortStableFunc(permutation, func(a, b int) int {
			x, y := items[a], items[b]
			if order.Descending {
				x, y = y, x
			}
			return compare(x, y)
		})
	})

	moved := selected
	sorted := make([]T, len(permutation))
	for at, from := range permutation {
		sorted[at] = items[from]
		if from == selected {
			moved = at
		}
	}
	return sorted, moved
}

func (t *Table[T]) sortAction() scene.ActionBinding {
	return scene.NewAction(scene.ActionConfig[SortOrder, *SortOrder]{
		ID: ActionSort, Label: "Set sort order",
		Availability: func(order SortOrder) scene.Availability {
			if !t.canSort(order.Column) {
				return scene.Availability{Disabled: true, Reason: "sort order is not available"}
			}
			return scene.Availability{}
		},
		Run: func(_ context.Context, order SortOrder) (scene.ActionResult[*SortOrder], error) {
			state := scene.Unchanged
			if t.SetSort(order) {
				state = scene.Applied
			}
			var current *SortOrder
			if t.order != nil {
				orderSnapshot := *t.order
				current = &orderSnapshot
			}
			return scene.ActionResult[*SortOrder]{State: state, Value: current}, nil
		},
	}).Binding()
}

func (t *Table[T]) clearSortAction() scene.ActionBinding {
	return scene.NewAction(scene.ActionConfig[struct{}, struct{}]{
		ID: ActionClearSort, Label: "Clear sort order",
		Run: func(context.Context, struct{}) (scene.ActionResult[struct{}], error) {
			state := scene.Unchanged
			if t.order != nil {
				t.ClearSort()
				state = scene.Applied
			}
			return scene.ActionResult[struct{}]{State: state}, nil
		},
	}).Binding()
}

func (t *Table[T]) Row() func(grid.View, int, T, bool) { return t.rows.Row() }

func (t *Table[T]) SetRow(value func(grid.View, int, T, bool)) {
	t.Element()
	t.rows.SetRow(value)
}

func (t *Table[T]) Keys() *keymap.Map { return t.rows.Keys() }

func (t *Table[T]) SetKeys(value *keymap.Map) {
	t.Element()
	t.rows.SetKeys(value)
}

func (t *Table[T]) Wrap() bool { return t.rows.Wrap() }

func (t *Table[T]) SetWrap(value bool) {
	t.Element()
	t.rows.SetWrap(value)
}
