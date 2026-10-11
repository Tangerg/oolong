package headless_test

import (
	"slices"
	"testing"

	"github.com/Tangerg/oolong/components/headless"
)

func TestTableComparisonFailureLeavesThePublishedOrderIntact(t *testing.T) {
	for _, operation := range []string{"first sort", "reverse", "comparison", "rows"} {
		t.Run(operation, func(t *testing.T) {
			var table headless.Table[int]
			fail := false
			table.SetLess(func(a, b int, _ int) bool {
				if fail {
					panic("comparison failed")
				}
				return a < b
			})
			table.SetItems([]int{3, 1, 2})
			if operation != "first sort" {
				table.SortBy(0)
			}
			table.Select(1)
			before := table.Items()
			column, descending, sorted := table.Sorted()
			func() {
				defer func() {
					if got := recover(); got != "comparison failed" {
						t.Fatalf("comparison panic = %v", got)
					}
				}()
				switch operation {
				case "first sort", "reverse":
					fail = true
					table.SortBy(0)
				case "comparison":
					table.SetLess(func(int, int, int) bool { panic("comparison failed") })
				case "rows":
					fail = true
					table.SetItems([]int{9, 7, 8})
				}
			}()
			gotColumn, gotDescending, gotSorted := table.Sorted()
			if !slices.Equal(table.Items(), before) || table.Selected() != 1 ||
				gotColumn != column || gotDescending != descending || gotSorted != sorted {
				t.Fatalf("failed operation changed publication: rows=%v cursor=%d order=(%d,%t,%t)",
					table.Items(), table.Selected(), gotColumn, gotDescending, gotSorted)
			}
			fail = false
			table.SortBy(0)
			want := []int{3, 2, 1}
			if !sorted {
				want = []int{1, 2, 3}
			}
			if !slices.Equal(table.Items(), want) {
				t.Fatalf("retry rows = %v, want %v", table.Items(), want)
			}
		})
	}
}

func TestTableComparisonCannotReplaceItsOwnOrderingInputs(t *testing.T) {
	for _, operation := range []string{"rows", "comparison", "sort", "clear"} {
		t.Run(operation, func(t *testing.T) {
			var table headless.Table[int]
			table.SetItems([]int{3, 1, 2})
			entered := false
			table.SetLess(func(a, b int, _ int) bool {
				if !entered {
					entered = true
					switch operation {
					case "rows":
						table.SetItems([]int{6, 5, 4})
					case "comparison":
						table.SetLess(nil)
					case "sort":
						table.SortBy(1)
					case "clear":
						table.ClearSort()
					}
				}
				return a < b
			})
			func() {
				defer func() {
					if recover() == nil {
						t.Error("comparison independently changed the ordering inputs")
					}
				}()
				table.SortBy(0)
			}()
			if _, _, sorted := table.Sorted(); sorted || !slices.Equal(table.Items(), []int{3, 1, 2}) {
				t.Fatalf("interrupted comparison changed rows or order: %v sorted=%t", table.Items(), sorted)
			}
			table.SortBy(0)
			if !slices.Equal(table.Items(), []int{1, 2, 3}) {
				t.Fatalf("table could not sort after interruption: %v", table.Items())
			}
		})
	}
}

func TestTableComparisonReadsTheLastCompleteOrder(t *testing.T) {
	var table headless.Table[int]
	table.SetItems([]int{3, 1, 2})
	checking := false
	table.SetLess(func(a, b int, _ int) bool {
		if checking {
			_, descending, sorted := table.Sorted()
			if !sorted || descending || !slices.Equal(table.Items(), []int{1, 2, 3}) {
				t.Fatal("comparison observed partial ordering publication")
			}
		}
		return a < b
	})
	table.SortBy(0)
	checking = true
	table.SortBy(0)
}
