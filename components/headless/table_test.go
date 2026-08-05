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
	t := &headless.Table[row]{
		Less: func(a, b row, column int) bool {
			if column == 1 {
				return a.size < b.size
			}
			return a.name < b.name
		},
	}
	t.SetRows([]row{{"gamma", 2}, {"alpha", 30}, {"beta", 1}})
	return t
}

func ordered(t *headless.Table[row]) string {
	out := make([]string, 0, len(t.Items))
	for _, r := range t.Items {
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
	paint(20, 3, tbl.Draw)
	tbl.Select(2) // beta
	tbl.SortBy(0)
	if got, _ := tbl.Current(); got.name != "beta" {
		t.Fatalf("after sorting the cursor is on %q", got.name)
	}
}

func TestReplacingTheRowsKeepsTheOrder(t *testing.T) {
	// A table refreshed while it is being read must not jump back into the order the
	// rows happened to arrive in.
	tbl := table()
	tbl.SortBy(0)
	tbl.SetRows([]row{{"delta", 9}, {"charlie", 8}})
	if got := ordered(tbl); got != "charlie delta" {
		t.Fatalf("the replaced rows are %q", got)
	}
}

func TestATableWithNoComparisonCannotBeSorted(t *testing.T) {
	// Rows that arrive in an order that means something — a log, a queue — are left
	// exactly as they came.
	var tbl headless.Table[row]
	tbl.SetRows([]row{{"b", 1}, {"a", 2}})
	if tbl.SortBy(0) {
		t.Fatal("a table with nothing to compare with sorted itself")
	}
	if got := ordered(&tbl); got != "b a" {
		t.Fatalf("the rows are %q", got)
	}
}
