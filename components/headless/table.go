package headless

import (
	"slices"

	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/keymap"
)

// Table is a list of rows with more than one column: a cursor, a window onto more
// rows than fit, and an order.
//
// It is a [List] inside, because moving a selection, keeping it in view, taking the
// wheel and answering a click is the same question in one column as in six. What a
// table adds is which column it is sorted by.
//
// The list is held rather than embedded, and the difference is the order: a table's
// rows are in the order the table decided, so an embedded list would hand every caller
// a second way to replace them and leave the header claiming an order the rows are no
// longer in.
//
// Where the columns are is not here either. A row is drawn by [List.Row] into a view
// of the whole row, and how that row is divided belongs to its appearance layer.
//
// The zero Table is an empty list in no particular order. A Table must not be copied
// after first use: its rows, cursor, ordering and scroll are one mutable owner.
// Comparisons must be pure. Reentering SetLess, SetItems, SortBy or ClearSort from
// a comparison panics; see SetLess for ordering and failure guarantees.
type Table[T any] struct {
	noCopy noCopy

	// Row, Keys and Wrap configure the rows — see the [List] fields of the same
	// names. They live here because the list does not belong to the caller.
	Row  func(v grid.View, at int, item T, selected bool)
	Keys *keymap.Map
	Wrap bool

	// rows owns the cursor, the window and the order this table put them in.
	rows List[T]

	// less orders two rows by a column: true when a comes before b. Nil means the
	// table cannot be sorted, which is the right answer for rows that arrive in an
	// order that means something.
	//
	// It takes the column rather than being one function per column, because a table
	// sorted by a column the caller has already named is a table whose comparison is
	// a switch — and a switch written once beats a slice of functions that has to be
	// kept the same length as the columns.
	less func(a, b T, column int) bool

	order   *tableOrder
	sorting bool
}

type tableOrder struct {
	column     int
	descending bool
}

// SetLess changes how columns order rows. A table already in a sorted state is
// immediately reordered by the new comparison; nil leaves the current row order in
// place and marks it unsorted. Keeping this transition inside Table prevents its
// reported order from getting out of step with its rows.
// The comparison must be pure and define a strict weak ordering. Reentering an
// ordering mutation (SetLess, SetItems, SortBy or ClearSort) from it panics.
// A comparison panic leaves the previous rows, comparison and reported order intact.
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

// SortBy orders the rows by a column, and reports whether anything changed.
//
// Asking for the column it is already sorted by turns the order round, which is what
// a reader means by pressing the same header twice.
// Rows and their reported order publish together only after comparison succeeds.
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

// Sorted is the column the rows are in the order of, whether that order is reversed,
// and whether they are sorted at all. An unsorted table returns (0, false, false).
//
// It is what a header asks to draw the mark beside the column being sorted by, which
// is the only way a reader can tell an order from a coincidence.
func (t *Table[T]) Sorted() (column int, descending, ok bool) {
	if t.order == nil {
		return 0, false, false
	}
	return t.order.column, t.order.descending, true
}

// ClearSort forgets the order, leaving the rows where they are. It is what a caller
// calls when it has replaced the rows with something whose order means something.
func (t *Table[T]) ClearSort() {
	t.requireIdle()
	t.order = nil
}

// SetItems replaces the rows, keeping the order the table is sorted by.
//
// It is [List.SetItems] with the sort applied, under the same name on purpose:
// there is one way to give a table its rows, and it cannot be the one that quietly
// throws the order away. A table that lost its order every time its rows were
// refreshed would be a table nobody could read while it was updating.
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

// The operations below are the list's, forwarded. Reading rows and moving a cursor
// cannot disturb an order, so they pass straight through; what does not pass through
// is anything that could replace the rows.

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
func (t *Table[T]) Move(n int) { t.list().Move(n) }

// Scroll is the table's position, for a scrollbar drawn beside it.
func (t *Table[T]) Scroll() *Scroll { return t.list().Scroll() }

// Focus takes the keyboard, or gives it up — see [List.Focus].
func (t *Table[T]) Focus(has bool) { t.list().Focus(has) }

// Focused reports whether this table has the keyboard.
func (t *Table[T]) Focused() bool { return t.list().Focused() }

// Handle answers the keys, the wheel and a press that move the cursor.
func (t *Table[T]) Handle(ev input.Event) bool { return t.list().Handle(ev) }

// Do runs one of the list's actions by name. See [Doer].
func (t *Table[T]) Do(action keymap.Action) bool { return t.list().Do(action) }

// HeightForWidth is one row per row.
func (t *Table[T]) HeightForWidth(width int) int { return t.list().HeightForWidth(width) }

// Draw paints the rows that fit.
func (t *Table[T]) Draw(v Frame) { t.list().Draw(v) }

// DrawRows paints the rows that fit with a caller's own row painter — see
// [List.DrawRows].
func (t *Table[T]) DrawRows(v Frame, draw func(grid.View, int, T, bool)) {
	t.list().DrawRows(v, draw)
}

// list is the rows, with this table's configuration in them. Every operation
// forwarded from this type goes through it, and that is what it is for.
//
// The table's exported fields own what wrapping, which keys and which row painter
// mean; the list keeps its own copy of them to work from. Copying them at the
// operations that were thought to need it is a rule somebody has to keep, and the
// one that was missed was cursor movement: a table told to wrap did not wrap until
// something had drawn it. Going through here instead leaves no forwarded operation
// that can read the copy a frame behind its owner.
func (t *Table[T]) list() *List[T] {
	t.rows.Row, t.rows.Keys, t.rows.Wrap = t.Row, t.Keys, t.Wrap
	return &t.rows
}

func (t *Table[T]) requireIdle() {
	if t.sorting {
		panic("headless: table ordering mutation during comparison")
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
	// A stable sort, so rows the column cannot tell apart keep the order they were
	// given — which is where the caller's own idea of importance lives.
	//
	// The comparison is asked both ways round because the predicate answers one of
	// them and a sort needs all three: rows it cannot separate must compare equal,
	// or the sort has no ties to keep the order of.
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
