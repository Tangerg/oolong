package base_test

import (
	"cmp"
	"testing"

	"github.com/Tangerg/oolong/ui/base"
)

func TestReplacingItemsKeepsTheCursorOnTheSameItem(t *testing.T) {
	for _, test := range []struct {
		name string
		same func(a, b string) bool
		want string
	}{
		{name: "identity", same: base.Equal[string], want: "b"},
		{name: "index", want: "a"},
	} {
		t.Run(test.name, func(t *testing.T) {
			list := base.NewList(base.ListConfig[string]{Items: []string{"a", "b", "c"}, Same: test.same})
			list.Select(1)
			list.SetItems([]string{"new", "a", "b", "c"})
			if got, _ := list.Current(); got != test.want {
				t.Fatalf("cursor on %q, want %q", got, test.want)
			}
		})
	}
}

func TestAFilterFollowsItsItemAcrossASourceRefreshButNotANewPattern(t *testing.T) {
	text := func(s string) string { return s }
	filter := base.NewFilter(base.FilterConfig[string]{Same: base.Equal[string]})
	filter.SetItems([]string{"alpha", "beta", "gamma"}, text)
	filter.Select(2)
	filter.SetItems([]string{"delta", "gamma", "alpha", "beta"}, text)
	if got, _ := filter.Current(); got != "gamma" {
		t.Fatalf("refresh moved the cursor to %q, want gamma", got)
	}
	filter.SetPattern("a")
	if filter.Selected() != 0 {
		t.Fatalf("a new pattern left the cursor at %d, want the top", filter.Selected())
	}
}

func TestATableFollowsItsItemThroughItsSort(t *testing.T) {
	table := base.NewTable(base.TableConfig[int]{Items: []int{3, 1, 2}, Same: base.Equal[int]})
	table.SetSortColumns(func(int) func(a, b int) int { return cmp.Compare[int] })
	table.SortBy(0)
	table.Select(2)
	table.SetItems([]int{0, 3, 2, 1})
	if got, _ := table.Current(); got != 3 {
		t.Fatalf("cursor on %d after replacement, want 3", got)
	}
}
