package base_test

import (
	"cmp"
	"errors"
	"slices"
	"testing"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/internal/scenetest"
)

func TestTableSortAvailabilityComesFromTheColumnComparator(t *testing.T) {
	scenes := scenetest.New()
	t.Cleanup(scenes.Close)

	comparisons := 0
	columns := []func(int, int) int{
		func(a, b int) int { comparisons++; return cmp.Compare(a, b) },
		nil,
	}
	var table base.Table[int]
	table.SetSortColumns(func(column int) func(int, int) int {
		if column < 0 || column >= len(columns) {
			return nil
		}
		return columns[column]
	})
	table.SetItems([]int{2, 1})
	fixture := scenes.Mount(&table)
	t.Cleanup(fixture.Root.Close)
	for _, column := range []int{-1, 1, len(columns)} {
		order := base.SortOrder{Column: column}
		if table.SetSort(order) {
			t.Fatalf("unsupported column %d changed sort order", column)
		}
		result, err := fixture.Invoke(t.Context(), table.Element().Target(), base.ActionSort, order)
		if !errors.Is(err, scene.ErrUnavailable) || result.State != scene.Unchanged {
			t.Fatalf("column %d: result=%+v error=%v", column, result, err)
		}
	}
	if comparisons != 0 || !slices.Equal(table.Items(), []int{2, 1}) {
		t.Fatal("invalid sort parameters reached the comparator")
	}
	result, err := fixture.Invoke(t.Context(), table.Element().Target(), base.ActionSort, base.SortOrder{})
	if err != nil || result.State != scene.Applied || !slices.Equal(table.Items(), []int{1, 2}) {
		t.Fatalf("supported column: %+v, %v, rows=%v", result, err, table.Items())
	}
}

func TestReplacingSortColumnsRevokesAnUnavailableActiveOrder(t *testing.T) {
	var table base.Table[int]
	table.SetSortColumns(func(column int) func(int, int) int {
		if column == 0 {
			return cmp.Compare[int]
		}
		return nil
	})
	table.SetItems([]int{3, 1, 2})
	table.SortBy(0)
	table.SetSortColumns(func(column int) func(int, int) int {
		if column == 1 {
			return func(a, b int) int { return cmp.Compare(b, a) }
		}
		return nil
	})
	if _, _, sorted := table.Sorted(); sorted || !slices.Equal(table.Items(), []int{1, 2, 3}) {
		t.Fatal("replaced column capability retained unsupported metadata or reordered rows")
	}
	if table.SortBy(0) || !table.SortBy(1) || !slices.Equal(table.Items(), []int{3, 2, 1}) {
		t.Fatal("replacement column capability was not the sole sort authority")
	}
}

func TestSortColumnLookupCannotMutateTheOrderItAuthorizes(t *testing.T) {
	var table base.Table[int]
	table.SetItems([]int{2, 1})
	table.SetSortColumns(func(int) func(int, int) int {
		table.ClearSort()
		return cmp.Compare[int]
	})
	func() {
		defer func() {
			if recover() == nil {
				t.Error("column lookup mutated its own sort boundary")
			}
		}()
		table.SortBy(0)
	}()
	if _, _, sorted := table.Sorted(); sorted || !slices.Equal(table.Items(), []int{2, 1}) {
		t.Fatal("reentrant column lookup published a partial sort")
	}
}
