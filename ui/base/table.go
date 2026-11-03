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

	// Row callbacks must be observationally pure; see [List.Row].
	Row  func(v grid.View, at int, item T, selected bool)
	Keys *keymap.Map
	Wrap bool

	rows List[T]

	less func(a, b T, column int) bool

	order *SortOrder
}

type SortOrder struct {
	Column     int
	Descending bool
}

const (
	ActionSort      scene.ActionID = "sort"
	ActionClearSort scene.ActionID = "clear-sort"
)

// SetLess installs a pure strict weak ordering and reapplies the active sort. Nil
// clears the sort without moving rows. Reentrant ordering mutations panic; comparison
// panics leave rows, comparator and reported order intact.
// Column interpretation belongs to the comparator because this row controller owns
// no column schema; callers must use indices supported by that comparator.
func (t *Table[T]) SetLess(less func(a, b T, column int) bool) {
	requireMutable(t.element)
	if less == nil {
		t.less = nil
		t.ClearSort()
		return
	}
	if t.order == nil {
		t.less = less
		return
	}
	items, selected := t.sortRows(t.rows.Items(), t.rows.Selected(), less, *t.order)
	t.rows.matcher.Reset(func() {
		t.less = less
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
	if !t.canSort(order.Column) || t.order != nil && *t.order == order {
		return false
	}
	items, selected := t.sortRows(t.rows.Items(), t.rows.Selected(), t.less, order)
	t.rows.matcher.Reset(func() {
		t.order = &order
		t.replaceRows(items, selected)
	})
	return true
}

func (t *Table[T]) canSort(column int) bool {
	return t.less != nil && column >= 0 && t.rows.Len() > 0
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

// SetItems replaces rows and reapplies the active sort.
func (t *Table[T]) SetItems(items []T) {
	requireMutable(t.element)
	items = slices.Clone(items)
	selected := min(max(t.rows.selected, 0), max(len(items)-1, 0))
	if t.order != nil {
		items, selected = t.sortRows(items, selected, t.less, *t.order)
	}
	t.rows.matcher.Reset(func() {
		t.replaceRows(items, selected)
	})
}

func (t *Table[T]) Items() []T { return t.list().Items() }

func (t *Table[T]) Len() int { return t.list().Len() }

func (t *Table[T]) At(index int) (T, bool) { return t.list().At(index) }

// Selected is the index of the row under the cursor, or -1.
func (t *Table[T]) Selected() int { return t.list().Selected() }

func (t *Table[T]) Current() (T, bool) { return t.list().Current() }

// Select puts the cursor on a row.
func (t *Table[T]) Select(i int) { t.list().Select(i) }

// Move steps the cursor by n rows.
func (t *Table[T]) Move(n int) { t.configureRows(); t.rows.Move(n) }

func (t *Table[T]) Scroll() *Scroll { return t.list().Scroll() }

func (t *Table[T]) focusChanged(has bool) { t.list().focusChanged(has) }

func (t *Table[T]) Focused() bool { return t.list().Focused() }

// Handle answers the keys, the wheel and a press that move the cursor.
func (t *Table[T]) Handle(ev input.Event) bool { t.configureRows(); return t.rows.Handle(ev) }

func (t *Table[T]) Do(action keymap.Action) bool { t.configureRows(); return t.rows.Do(action) }

func (t *Table[T]) HeightForWidth(width int) int { return t.list().HeightForWidth(width) }

func (t *Table[T]) Draw(v scene.Frame) { t.rows.DrawRows(v, t.Row) }

// DrawRows paints the rows that fit with a caller's own row painter — see
// [List.DrawRows].
func (t *Table[T]) DrawRows(v scene.Frame, draw func(grid.View, int, T, bool)) {
	t.list().DrawRows(v, draw)
}

func (t *Table[T]) list() *List[T] { return &t.rows }

func (t *Table[T]) configureRows() {
	requireMutable(t.element)
	t.rows.Keys, t.rows.Wrap = t.Keys, t.Wrap
}

func (t *Table[T]) replaceRows(items []T, selected int) {
	t.rows.replaceItems(items, selected)
	t.rows.notifyChange()
}

// Sorting a permutation preserves the selected row without requiring T equality.
// Preparation borrows only private rows; publication and resolver cancellation wait
// until the comparison has returned, so a failure cannot claim an unfinished order.
func (t *Table[T]) sortRows(items []T, selected int, less func(T, T, int) bool, order SortOrder) ([]T, int) {
	if len(items) < 2 {
		return items, selected
	}

	permutation := make([]int, len(items))
	for i := range permutation {
		permutation[i] = i
	}
	// Compare both directions to preserve ties and caller order in the stable sort.
	t.Element().ReadOnly(func() {
		slices.SortStableFunc(permutation, func(a, b int) int {
			x, y := items[a], items[b]
			if order.Descending {
				x, y = y, x
			}
			switch {
			case less(x, y, order.Column):
				return -1
			case less(y, x, order.Column):
				return 1
			default:
				return 0
			}
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
