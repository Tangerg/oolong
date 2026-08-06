package headless

import "slices"

// Table is a list of rows with more than one column: a cursor, a window onto more
// rows than fit, and an order.
//
// It is a [List] and says so — everything about moving a selection, keeping it in
// view, taking the wheel and answering a click is the same question in one column as
// in six, and a table that answered it again would be a second place for it to be
// wrong. What a table has that a list does not is which column it is sorted by, so
// that is all this adds.
//
// Where the columns are is not here either. A row is drawn by [List.Row] into a view
// of the whole row, and how that row is divided is the business of whatever draws
// it — see the table in the kit package, which takes the geometry and the header.
//
// The zero Table is an empty list in no particular order.
type Table[T any] struct {
	List[T]

	// Less orders two rows by a column: true when a comes before b. Nil means the
	// table cannot be sorted, which is the right answer for rows that arrive in an
	// order that means something.
	//
	// It takes the column rather than being one function per column, because a table
	// sorted by a column the caller has already named is a table whose comparison is
	// a switch — and a switch written once beats a slice of functions that has to be
	// kept the same length as the columns.
	Less func(a, b T, column int) bool

	column     int
	descending bool
	sorted     bool
}

// SortBy orders the rows by a column, and reports whether anything changed.
//
// Asking for the column it is already sorted by turns the order round, which is what
// a reader means by pressing the same header twice.
func (t *Table[T]) SortBy(column int) bool {
	if t.Less == nil || column < 0 || len(t.Items) == 0 {
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

// Unsorted forgets the order, leaving the rows where they are. It is what a caller
// calls when it has replaced the rows with something whose order means something.
func (t *Table[T]) Unsorted() { t.sorted = false }

// SetItems replaces the rows, keeping the order the table is sorted by.
//
// It is [List.SetItems] with the sort applied, under the same name on purpose:
// there is one way to give a table its rows, and it cannot be the one that quietly
// throws the order away. A table that lost its order every time its rows were
// refreshed would be a table nobody could read while it was updating.
func (t *Table[T]) SetItems(items []T) {
	t.List.SetItems(items)
	t.reorder()
}

// reorder sorts the rows and carries the cursor with the row it was on.
//
// The permutation is sorted rather than the rows, so that where the selected row
// went is known exactly. Following it by comparing rows afterwards would need this
// type to know how to tell two of them apart, which is knowledge only the caller
// has — and would land on the wrong row whenever two of them were alike.
func (t *Table[T]) reorder() {
	if !t.sorted || t.Less == nil || len(t.Items) < 2 {
		return
	}
	order := make([]int, len(t.Items))
	for i := range order {
		order[i] = i
	}
	// A stable sort, so rows the column cannot tell apart keep the order they were
	// given — which is where the caller's own idea of importance lives.
	//
	// The comparison is asked both ways round because [Table.Less] answers one of
	// them and a sort needs all three: rows it cannot separate must compare equal,
	// or the sort has no ties to keep the order of.
	slices.SortStableFunc(order, func(a, b int) int {
		x, y := t.Items[a], t.Items[b]
		if t.descending {
			x, y = y, x
		}
		switch {
		case t.Less(x, y, t.column):
			return -1
		case t.Less(y, x, t.column):
			return 1
		default:
			return 0
		}
	})

	was := t.Selected()
	moved := was
	items := make([]T, len(order))
	for at, from := range order {
		items[at] = t.Items[from]
		if from == was {
			moved = at
		}
	}
	t.Items = items
	if moved >= 0 {
		t.Select(moved)
	}
}
