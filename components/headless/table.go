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

	column     int
	descending bool
	sorted     bool
}

// SetLess changes how columns order rows. A table already in a sorted state is
// immediately reordered by the new comparison; nil leaves the current row order in
// place and marks it unsorted. Keeping this transition inside Table prevents its
// reported order from getting out of step with its rows.
func (t *Table[T]) SetLess(less func(a, b T, column int) bool) {
	t.less = less
	if less == nil {
		t.sorted = false
		return
	}
	t.reorder()
}

// SortBy orders the rows by a column, and reports whether anything changed.
//
// Asking for the column it is already sorted by turns the order round, which is what
// a reader means by pressing the same header twice.
func (t *Table[T]) SortBy(column int) bool {
	if t.less == nil || column < 0 || t.rows.Len() == 0 {
		return false
	}
	if t.sorted && t.column == column {
		t.descending = !t.descending
	} else {
		t.column, t.descending, t.sorted = column, false, true
	}
	t.reorder()
	return true
}

// Sorted is the column the rows are in the order of, whether that order is reversed,
// and whether they are sorted at all.
//
// It is what a header asks to draw the mark beside the column being sorted by, which
// is the only way a reader can tell an order from a coincidence.
func (t *Table[T]) Sorted() (column int, descending, ok bool) {
	return t.column, t.descending, t.sorted
}

// ClearSort forgets the order, leaving the rows where they are. It is what a caller
// calls when it has replaced the rows with something whose order means something.
func (t *Table[T]) ClearSort() { t.sorted = false }

// SetItems replaces the rows, keeping the order the table is sorted by.
//
// It is [List.SetItems] with the sort applied, under the same name on purpose:
// there is one way to give a table its rows, and it cannot be the one that quietly
// throws the order away. A table that lost its order every time its rows were
// refreshed would be a table nobody could read while it was updating.
func (t *Table[T]) SetItems(items []T) {
	t.list().SetItems(items)
	t.reorder()
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

// reorder sorts the rows and carries the cursor with the row it was on.
//
// The permutation is sorted rather than the rows, so that where the selected row
// went is known exactly. Following it by comparing rows afterwards would need this
// type to know how to tell two of them apart, which is knowledge only the caller
// has — and would land on the wrong row whenever two of them were alike.
func (t *Table[T]) reorder() {
	if !t.sorted || t.less == nil || t.rows.Len() < 2 {
		return
	}
	items := t.rows.Items()
	order := make([]int, len(items))
	for i := range order {
		order[i] = i
	}
	// A stable sort, so rows the column cannot tell apart keep the order they were
	// given — which is where the caller's own idea of importance lives.
	//
	// The comparison is asked both ways round because the predicate answers one of
	// them and a sort needs all three: rows it cannot separate must compare equal,
	// or the sort has no ties to keep the order of.
	slices.SortStableFunc(order, func(a, b int) int {
		x, y := items[a], items[b]
		if t.descending {
			x, y = y, x
		}
		switch {
		case t.less(x, y, t.column):
			return -1
		case t.less(y, x, t.column):
			return 1
		default:
			return 0
		}
	})

	was := t.rows.Selected()
	moved := was
	sorted := make([]T, len(order))
	for at, from := range order {
		sorted[at] = items[from]
		if from == was {
			moved = at
		}
	}
	t.rows.SetItems(sorted)
	if moved >= 0 {
		t.rows.Select(moved)
	}
}
