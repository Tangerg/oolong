package base

import (
	"context"
	"slices"
	"strconv"

	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/scene"
)

// Table owns rows and their sort order. Its SetItems reapplies the active sort, so
// replacement cannot bypass sorting; appearance owns column geometry. Navigation,
// hit testing and subscription are a [List]'s. The zero value is ready.
// A Table must not be copied after first use. Comparisons must be pure; reentrant
// ordering mutations panic and failed comparisons preserve the previous state.
// Exhausting the collection revision panics before changing rows or sort metadata.
type Table[T any] struct {
	rows[T]
	noCopy noCopy

	sortColumns func(column int) func(a, b T) int

	order *SortOrder
}

type SortOrder struct {
	Column     int
	Descending bool
}

// TableConfig is a [ListConfig]: a table's rows take the same settings as a list's.
type TableConfig[T any] struct {
	ListConfig[T]
}

func NewTable[T any](config TableConfig[T]) *Table[T] {
	t := &Table[T]{}
	t.configure(config.ListConfig)
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
	items, selected := t.sortRows(t.Items(), t.Selected(), compare, *t.order)
	t.requireReplacement()
	t.matcher.Reset(func() {
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
// An empty table records the order, which applies to the rows it later receives.
func (t *Table[T]) SetSort(order SortOrder) bool {
	requireMutable(t.element)
	if t.order != nil && *t.order == order {
		return false
	}
	compare := t.columnComparator(t.sortColumns, order.Column)
	if compare == nil {
		return false
	}
	items, selected := t.sortRows(t.Items(), t.Selected(), compare, order)
	t.requireReplacement()
	t.matcher.Reset(func() {
		t.order = &order
		t.replaceRows(items, selected)
	})
	return true
}

func (t *Table[T]) canSort(column int) bool {
	return t.columnComparator(t.sortColumns, column) != nil
}

func (t *Table[T]) columnComparator(columns func(int) func(T, T) int, column int) func(T, T) int {
	if columns == nil || column < 0 {
		return nil
	}
	return readProjection(t.Element(), func() func(T, T) int { return columns(column) })
}

// Sorted returns the active order and whether the rows are sorted at all.
func (t *Table[T]) Sorted() (SortOrder, bool) {
	if t.order == nil {
		return SortOrder{}, false
	}
	return *t.order, true
}

// ClearSort clears sort metadata without moving rows and reports whether the
// rows were sorted.
func (t *Table[T]) ClearSort() bool {
	requireMutable(t.element)
	if t.order == nil {
		return false
	}
	t.order = nil
	t.element.Invalidate()
	return true
}

// SetItems reapplies the active sort before publishing rows. A lookup that revokes
// that column without SetSortColumns violates its stability contract and panics.
func (t *Table[T]) SetItems(items []T) {
	requireMutable(t.element)
	items = slices.Clone(items)
	selected := t.followed(items, min(max(t.selected, 0), max(len(items)-1, 0)))
	if t.order != nil {
		compare := t.columnComparator(t.sortColumns, t.order.Column)
		if compare == nil {
			panic("base: sort columns changed without SetSortColumns")
		}
		items, selected = t.sortRows(items, selected, compare, *t.order)
	}
	t.requireReplacement()
	t.matcher.Reset(func() {
		t.replaceRows(items, selected)
	})
}

func (t *Table[T]) replaceRows(items []T, selected int) {
	t.replaceItems(items, selected)
	t.notifyChange()
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
				return scene.Unavailable("sort order is not available")
			}
			return scene.Availability{}
		},
		Run: func(_ context.Context, order SortOrder) (scene.ActionResult[*SortOrder], error) {
			state := scene.AppliedIf(t.SetSort(order))
			var current *SortOrder
			if order, sorted := t.Sorted(); sorted {
				current = &order
			}
			return scene.ActionResult[*SortOrder]{State: state, Value: current}, nil
		},
	}).Binding()
}

func (t *Table[T]) clearSortAction() scene.ActionBinding {
	return scene.NewAction(scene.ActionConfig[struct{}, struct{}]{
		ID: ActionClearSort, Label: "Clear sort order",
		Run: func(context.Context, struct{}) (scene.ActionResult[struct{}], error) {
			return scene.ActionResult[struct{}]{State: scene.AppliedIf(t.ClearSort())}, nil
		},
	}).Binding()
}

func (t *Table[T]) Element() scene.Element {
	if t.element.IsZero() {
		t.element = scene.NewElement(scene.ElementConfig{
			Measure: layout.MeasureRows(t.HeightForWidth), Paint: t.Draw,
			Handle: func(event scene.Event) scene.EventResult {
				return partEvent(event, t.Handle, &t.geometry, &t.pointer.geometry)
			},
			Focusable: func() bool { return true }, FocusChanged: t.focusChanged,
			Semantics: func() scene.Semantic {
				return scene.Semantic{Role: scene.RoleTable, ContentRevision: t.contentRevision, Value: strconv.Itoa(t.Selected())}
			}, Actions: append(collectionActions(&t.rows, unchangedItem[T]), collectionActivateAction(&t.rows), t.sortAction(), t.clearSortAction()),
		})
	}
	t.adopt(t.element)
	return t.element
}
