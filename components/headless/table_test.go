package headless_test

import (
	"strings"
	"testing"

	"github.com/Tangerg/oolong/components/headless"
)

// row is a table row of two columns, so a test can sort by either.
type row struct {
	name string
	size int
}

func table() *headless.Table[row] {
	t := new(headless.Table[row])
	t.SetLess(func(a, b row, column int) bool {
		if column == 1 {
			return a.size < b.size
		}
		return a.name < b.name
	})
	t.SetItems([]row{{"gamma", 2}, {"alpha", 30}, {"beta", 1}})
	return t
}

func TestChangingTheComparisonKeepsTheReportedOrderTrue(t *testing.T) {
	tbl := table()
	tbl.SortBy(0)
	tbl.SetLess(func(a, b row, _ int) bool { return a.size < b.size })
	if got := ordered(tbl); got != "beta gamma alpha" {
		t.Fatalf("rows after changing the comparison are %q", got)
	}
	if column, descending, ok := tbl.Sorted(); !ok || column != 0 || descending {
		t.Fatalf("table reports column %d, descending %v, sorted %v", column, descending, ok)
	}

	tbl.SetLess(nil)
	if _, _, ok := tbl.Sorted(); ok {
		t.Fatal("table without a comparison still reports a sorted order")
	}
}

func ordered(t *headless.Table[row]) string {
	out := make([]string, 0, t.Len())
	for _, r := range t.Items() {
		out = append(out, r.name)
	}
	return strings.Join(out, " ")
}

func TestATableIsOrderedByTheColumnItWasAsked(t *testing.T) {
	tbl := table()
	if got := ordered(tbl); got != "gamma alpha beta" {
		t.Fatalf("rows nobody sorted are %q", got)
	}
	tbl.SortBy(0)
	if got := ordered(tbl); got != "alpha beta gamma" {
		t.Fatalf("sorted by name they are %q", got)
	}
	// The same column again turns it round, which is what pressing a header twice
	// means.
	tbl.SortBy(0)
	if got := ordered(tbl); got != "gamma beta alpha" {
		t.Fatalf("sorted the other way they are %q", got)
	}
	if column, descending, ok := tbl.Sorted(); !ok || column != 0 || !descending {
		t.Fatalf("the table says it is sorted by %d, reversed %v, at all %v", column, descending, ok)
	}
	tbl.SortBy(1)
	if got := ordered(tbl); got != "beta gamma alpha" {
		t.Fatalf("sorted by size they are %q", got)
	}
}

func TestTheCursorStaysOnTheRowItWasOnThroughASort(t *testing.T) {
	// Following it by index would land on whatever moved into that position, which is
	// how a reader acts on the wrong row.
	tbl := table()
	paintWidget(20, 3, tbl)
	tbl.Select(2) // beta
	tbl.SortBy(0)
	if got, _ := tbl.Current(); got.name != "beta" {
		t.Fatalf("after sorting the cursor is on %q", got.name)
	}
}

func TestReplacingTheRowsKeepsTheOrder(t *testing.T) {
	// A table refreshed while it is being read must not jump back into the order the
	// rows happened to arrive in — and it is the list's own method that does it, so
	// there is no second way to set the rows that forgets.
	tbl := table()
	tbl.SortBy(0)
	tbl.SetItems([]row{{"delta", 9}, {"charlie", 8}})
	if got := ordered(tbl); got != "charlie delta" {
		t.Fatalf("the replaced rows are %q", got)
	}

	// Until the order is given up, which is what a caller does when the rows arrive
	// in an order that means something.
	tbl.ClearSort()
	tbl.SetItems([]row{{"delta", 9}, {"charlie", 8}})
	if got := ordered(tbl); got != "delta charlie" {
		t.Fatalf("after giving up the order the rows are %q", got)
	}
	if _, _, ok := tbl.Sorted(); ok {
		t.Error("a table that gave up its order still says it has one")
	}
}

func TestATableWithNoComparisonCannotBeSorted(t *testing.T) {
	// Rows that arrive in an order that means something — a log, a queue — are left
	// exactly as they came.
	var tbl headless.Table[row]
	tbl.SetItems([]row{{"b", 1}, {"a", 2}})
	if tbl.SortBy(0) {
		t.Fatal("a table with nothing to compare with sorted itself")
	}
	if got := ordered(&tbl); got != "b a" {
		t.Fatalf("the rows are %q", got)
	}
}
