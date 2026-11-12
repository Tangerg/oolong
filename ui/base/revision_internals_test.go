package base

import (
	"cmp"
	"math"
	"slices"
	"testing"
)

func TestLocalConfirmationCannotReuseAnAnswerRevision(t *testing.T) {
	confirm := NewConfirm(ConfirmConfig{})
	owner := confirm.control.(*localConfirm)
	owner.answer.Revision = math.MaxUint64 - 1
	confirm.Say(true)
	before := confirm.Snapshot()
	if !before.Value || before.Revision != math.MaxUint64 {
		t.Fatalf("last revision = %+v", before)
	}
	if got := panicFrom(func() { confirm.Say(false) }); got == nil {
		t.Fatal("exhausted answer revision wrapped instead of refusing the change")
	}
	if after := confirm.Snapshot(); after != before {
		t.Fatalf("refused answer changed its snapshot: %+v -> %+v", before, after)
	}
	confirm.Say(true)
}

func TestChoicesRefuseRevisionExhaustionBeforeChangingAnswers(t *testing.T) {
	t.Run("select", func(t *testing.T) {
		field := NewSelect(SelectConfig[string]{Options: Options("one", "two"), Same: Equal[string]})
		field.revision = math.MaxUint64
		for _, change := range []func(){
			func() { field.Choose("two") },
			func() { field.SetOptions(Options("replacement")) },
		} {
			if panicFrom(change) == nil {
				t.Fatal("exhausted answer revision accepted a change")
			}
			chosen, _ := field.Chosen()
			if chosen.Value != "one" || !slices.Equal(field.Options(), Options("one", "two")) || field.Revision() != math.MaxUint64 {
				t.Fatal("refused change modified the answer or options")
			}
		}
		if !field.Choose("one") {
			t.Fatal("unchanged answer was refused")
		}
	})
	t.Run("multi-select", func(t *testing.T) {
		field := NewMultiSelect(MultiSelectConfig[string]{Options: Options("one", "two"), Same: Equal[string], Initial: []string{"one", "two"}})
		field.revision = math.MaxUint64
		for _, change := range []func(){
			func() { field.SetTaken([]string{"two"}) },
			func() { field.SetLimit(1) },
			func() { field.SetOptions(Options("replacement")) },
		} {
			if panicFrom(change) == nil {
				t.Fatal("exhausted answer revision accepted a change")
			}
			if !slices.Equal(field.Taken(), []string{"one", "two"}) || field.Limit() != 0 || !slices.Equal(field.Options(), Options("one", "two")) || field.Revision() != math.MaxUint64 {
				t.Fatal("refused change modified the answer, limit or options")
			}
		}
		field.SetTaken([]string{"one", "two"})
		field.SetLimit(3)
	})
}

func TestCollectionRevisionExhaustionCannotPartiallyChangeComposedOwners(t *testing.T) {
	t.Run("list", func(t *testing.T) {
		list := NewList(ListConfig[int]{Items: []int{1, 2}})
		list.contentRevision = math.MaxUint64
		if panicFrom(func() { list.SetItems([]int{3}) }) == nil || !slices.Equal(list.Items(), []int{1, 2}) {
			t.Fatal("exhausted collection revision replaced list items")
		}
		list.Select(1)
		if list.Selected() != 1 {
			t.Fatal("selection consumed a collection revision")
		}
	})
	t.Run("tree", func(t *testing.T) {
		tree := NewTree(Node[int]{Item: 1, Children: []Node[int]{{Item: 2}}})
		tree.list.contentRevision = math.MaxUint64
		for _, change := range []func(){
			func() { tree.Open(0) },
			func() { tree.SetNodes([]Node[int]{{Item: 3}}) },
		} {
			if panicFrom(change) == nil || len(tree.Rows()) != 1 || tree.Rows()[0].Open || tree.Nodes()[0].Item != 1 {
				t.Fatal("refused replacement changed the hierarchy or branch state")
			}
		}
	})
	t.Run("filter", func(t *testing.T) {
		filter := NewFilter(FilterConfig[string]{})
		filter.SetItems([]string{"one", "two"}, func(s string) string { return s })
		filter.list.contentRevision = math.MaxUint64
		for _, change := range []func(){
			func() { filter.SetPattern("one") },
			func() { filter.SetItems([]string{"replacement"}, nil) },
		} {
			if panicFrom(change) == nil || filter.Pattern() != "" || filter.Matched() != 2 || !slices.Equal(filter.Items(), []string{"one", "two"}) {
				t.Fatal("refused replacement changed the source, pattern or matches")
			}
		}
	})
	t.Run("table", func(t *testing.T) {
		table := NewTable(TableConfig[int]{Items: []int{2, 1}})
		table.SetSortColumns(func(int) func(int, int) int { return cmp.Compare[int] })
		table.rows.contentRevision = math.MaxUint64
		if panicFrom(func() { table.SortBy(0) }) == nil {
			t.Fatal("exhausted collection revision accepted a sort")
		}
		if _, _, sorted := table.Sorted(); sorted || !slices.Equal(table.Items(), []int{2, 1}) {
			t.Fatal("refused sort changed its order or rows")
		}
	})
	t.Run("completion", func(t *testing.T) {
		completion := NewCompletion(CompletionConfig{})
		completion.Offer(Token{Query: "one"}, []Candidate{{Text: "first"}})
		completion.list.contentRevision = math.MaxUint64
		for _, change := range []func(){
			func() { completion.Offer(Token{Query: "two"}, []Candidate{{Text: "second"}}) },
			func() { completion.Dismiss() },
		} {
			if panicFrom(change) == nil || completion.token.Query != "one" || completion.list.items[0].Text != "first" {
				t.Fatal("refused replacement changed the offer or its token")
			}
		}
	})
}
