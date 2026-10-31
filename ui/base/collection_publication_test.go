package base_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/scene"
)

func TestCollectionFocusProjectionReadsOnlyThePublishedRowsAndSelection(t *testing.T) {
	for _, kind := range []string{"list", "select", "multi", "table", "filter", "completion"} {
		t.Run(kind, func(t *testing.T) {
			var part scene.Element
			var change func()
			var complete func() bool
			switch kind {
			case "list":
				l := new(base.List[int])
				l.SetItems([]int{1, 2})
				l.Select(1)
				part, change = l.Element(), func() { l.SetItems([]int{3}) }
				complete = func() bool { n, ok := l.Current(); return ok && n == 3 && l.Selected() == 0 }
			case "select":
				s := base.NewSelect(base.SelectConfig[string]{Options: base.Options("a", "b"), Initial: "b", Same: base.Equal[string]})
				part, change = s.Element(), func() { s.SetOptions(base.Options("b", "a")) }
				complete = func() bool { n, ok := s.Chosen(); return ok && n.Value == "b" && s.Options()[0].Value == "b" }
			case "multi":
				m := base.NewMultiSelect(base.MultiSelectConfig[string]{Options: base.Options("a", "b"), Initial: []string{"b"}, Same: base.Equal[string]})
				part, change = m.Element(), func() { m.SetOptions(base.Options("b", "a")) }
				complete = func() bool { return slices.Equal(m.Taken(), []string{"b"}) && m.Options()[0].Value == "b" }
			case "table":
				b := new(base.Table[int])
				b.SetItems([]int{3, 1, 2})
				b.SetLess(func(a, b, _ int) bool { return a < b })
				part, change = b.Element(), func() { b.SortBy(0) }
				complete = func() bool {
					n, ok := b.Current()
					return ok && n == 3 && b.Selected() == 2 && slices.Equal(b.Items(), []int{1, 2, 3})
				}
			case "filter":
				f := new(base.Filter[string])
				f.SetItems([]string{"a", "b"}, func(v string) string { return v })
				f.Select(1)
				part, change = f.Element(), func() { f.SetPattern("a") }
				complete = func() bool { n, ok := f.Current(); return ok && n == "a" && f.Selected() == 0 }
			case "completion":
				c := new(base.Completion)
				c.Offer(base.Token{Query: "old"}, []base.Candidate{{Text: "old"}})
				part, change = c.Element(), func() { c.Offer(base.Token{Query: "new"}, []base.Candidate{{Text: "new"}}) }
				complete = func() bool {
					n, ok := c.Current()
					token, _ := c.Token()
					return ok && token.Query == "new" && n.Text == "new"
				}
			}
			checking, observed := false, false
			if err := part.SetFocusable(func() bool {
				observed = observed || checking
				if checking && !complete() {
					t.Error("focus predicate saw an incomplete collection publication")
				}
				return true
			}); err != nil {
				t.Fatal(err)
			}
			root, err := scene.NewRoot(scene.RootConfig{Element: part})
			if err != nil {
				t.Fatal(err)
			}
			defer root.Close()
			checking = true
			change()
			checking = false
			if !observed || !complete() {
				t.Fatalf("incomplete final state or missing publication (observed=%t)", observed)
			}
		})
	}
}

func TestCollectionSourceCommandsRejectProjectionBeforeChangingInputs(t *testing.T) {
	for _, kind := range []string{"table", "filter"} {
		t.Run(kind, func(t *testing.T) {
			var part scene.Element
			var change func()
			var unchanged func() bool
			if kind == "table" {
				b := new(base.Table[int])
				b.SetItems([]int{3, 1, 2})
				b.SetLess(func(a, b, _ int) bool { return a < b })
				b.SortBy(0)
				part, change, unchanged = b.Element(), b.ClearSort, func() bool { _, _, sorted := b.Sorted(); return sorted }
			} else {
				f := new(base.Filter[string])
				f.SetItems([]string{"a", "b"}, func(v string) string { return v })
				part, change, unchanged = f.Element(), func() { f.SetPattern("a") }, func() bool { return f.Pattern() == "" }
			}
			checking := false
			if err := part.SetFocusable(func() bool {
				if checking {
					change()
				}
				return true
			}); err != nil {
				t.Fatal(err)
			}
			root, err := scene.NewRoot(scene.RootConfig{Element: part})
			if err != nil {
				t.Fatal(err)
			}
			defer root.Close()
			checking = true
			func() {
				defer func() {
					got, _ := recover().(error)
					if !errors.Is(got, scene.ErrBusy) {
						t.Errorf("projection rejection = %v", got)
					}
				}()
				root.Changed()
			}()
			checking = false
			if !unchanged() {
				t.Error("rejected source command changed the model")
			}
		})
	}
}
