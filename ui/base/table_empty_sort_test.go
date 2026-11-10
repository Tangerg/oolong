package base_test

import (
	"cmp"
	"slices"
	"testing"

	"github.com/Tangerg/oolong/ui/base"
)

// A sort chosen before rows arrive used to be dropped, although an order that
// survives emptying the table already applied to later rows.
func TestAnEmptyTableKeepsTheSortItIsGiven(t *testing.T) {
	table := base.NewTable(base.TableConfig[int]{})
	table.SetSortColumns(func(int) func(a, b int) int { return cmp.Compare[int] })
	if !table.SetSort(base.SortOrder{Descending: true}) {
		t.Fatal("an empty table refused a sort")
	}
	if _, descending, ok := table.Sorted(); !ok || !descending {
		t.Fatal("the sort was not recorded")
	}
	table.SetItems([]int{1, 3, 2})
	if got := table.Items(); !slices.Equal(got, []int{3, 2, 1}) {
		t.Fatalf("items = %v, want them in the recorded order", got)
	}
}
