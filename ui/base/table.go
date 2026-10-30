package base

import (
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

	order   *tableOrder
	sorting bool
}

type tableOrder struct {
	column     int
	descending bool
}

// SetLess installs a pure strict weak ordering and reapplies the active sort. Nil
// clears the sort without moving rows. Reentrant ordering mutations panic; comparison
// panics leave rows, comparator and reported order intact.
func (t *Table[T]) SetLess(less func(a, b T, column int) bool) {
	t.requireIdle()
	if less == nil {
		t.less, t.order = nil, nil
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
	t.requireIdle()
	if t.less == nil || column < 0 || t.rows.Len() == 0 {
		return false
	}
	order := tableOrder{column: column}
	if t.order != nil && t.order.column == column {
		order.descending = !t.order.descending
	}
	items, selected := t.sortRows(t.rows.Items(), t.rows.Selected(), t.less, order)
	t.rows.matcher.Reset(func() {
		t.order = &order
		t.replaceRows(items, selected)
	})
	return true
}

// Sorted returns the sort column, reverse flag and whether sorting is active.
// An unsorted table returns (0, false, false).
func (t *Table[T]) Sorted() (column int, descending, ok bool) {
	if t.order == nil {
		return 0, false, false
	}
	return t.order.column, t.order.descending, true
}

// ClearSort clears sort metadata without moving rows.
func (t *Table[T]) ClearSort() {
	t.requireIdle()
	t.order = nil
}

// SetItems replaces rows and reapplies the active sort.
func (t *Table[T]) SetItems(items []T) {
	t.requireIdle()
	items = slices.Clone(items)
	selected := min(max(t.rows.selected, 0), max(len(items)-1, 0))
	if t.order != nil {
		items, selected = t.sortRows(items, selected, t.less, *t.order)
	}
	t.rows.matcher.Reset(func() {
		t.replaceRows(items, selected)
	})
}

// Items returns the rows in the order the table put them in.
func (t *Table[T]) Items() []T { return t.list().Items() }

// Len is how many rows there are.
func (t *Table[T]) Len() int { return t.list().Len() }

// At returns one row by index, and whether there is one.
func (t *Table[T]) At(index int) (T, bool) { return t.list().At(index) }

// Selected is the index of the row under the cursor, or -1.
func (t *Table[T]) Selected() int { return t.list().Selected() }

// Current is the row under the cursor, and whether there is one.
func (t *Table[T]) Current() (T, bool) { return t.list().Current() }

// Select puts the cursor on a row.
func (t *Table[T]) Select(i int) { t.list().Select(i) }

// Move steps the cursor by n rows.
func (t *Table[T]) Move(n int) { t.configureRows(); t.rows.Move(n) }

// Scroll is the table's position, for a scrollbar drawn beside it.
func (t *Table[T]) Scroll() *Scroll { return t.list().Scroll() }

// Focus takes the keyboard, or gives it up — see [List.Focus].
func (t *Table[T]) focusChanged(has bool) { t.list().focusChanged(has) }

// Focused reports whether this table has the keyboard.
func (t *Table[T]) Focused() bool { return t.list().Focused() }

// Handle answers the keys, the wheel and a press that move the cursor.
func (t *Table[T]) Handle(ev input.Event) bool { t.configureRows(); return t.rows.Handle(ev) }

// Do runs one of the list's actions by name. See [Doer].
func (t *Table[T]) Do(action keymap.Action) bool { t.configureRows(); return t.rows.Do(action) }

// HeightForWidth is one row per row.
func (t *Table[T]) HeightForWidth(width int) int { return t.list().HeightForWidth(width) }

// Draw paints the rows that fit.
func (t *Table[T]) Draw(v scene.Frame) { t.rows.DrawRows(v, t.Row) }

// DrawRows paints the rows that fit with a caller's own row painter — see
// [List.DrawRows].
func (t *Table[T]) DrawRows(v scene.Frame, draw func(grid.View, int, T, bool)) {
	t.list().DrawRows(v, draw)
}

// Project configuration before every List operation so movement cannot read stale
// wrapping or key bindings while waiting for the next draw.
func (t *Table[T]) list() *List[T] { return &t.rows }

func (t *Table[T]) configureRows() {
	requireMutable(t.element)
	t.rows.Keys, t.rows.Wrap = t.Keys, t.Wrap
}

func (t *Table[T]) requireIdle() {
	if t.sorting {
		panic("base: table ordering mutation during comparison")
	}
}

func (t *Table[T]) replaceRows(items []T, selected int) {
	t.list().SetItems(items)
	t.rows.Select(selected)
}

// Sorting a permutation preserves the selected row without requiring T equality.
// Preparation borrows only private rows; publication and resolver cancellation wait
// until the comparison has returned, so a failure cannot claim an unfinished order.
func (t *Table[T]) sortRows(items []T, selected int, less func(T, T, int) bool, order tableOrder) ([]T, int) {
	if len(items) < 2 {
		return items, selected
	}
	t.sorting = true
	defer func() { t.sorting = false }()
	permutation := make([]int, len(items))
	for i := range permutation {
		permutation[i] = i
	}
	// Compare both directions to preserve ties and caller order in the stable sort.
	slices.SortStableFunc(permutation, func(a, b int) int {
		x, y := items[a], items[b]
		if order.descending {
			x, y = y, x
		}
		switch {
		case less(x, y, order.column):
			return -1
		case less(y, x, order.column):
			return 1
		default:
			return 0
		}
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
