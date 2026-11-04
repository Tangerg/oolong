package base_test

import (
	"testing"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/input"
)

func TestTreeBranchChangesPreserveTheSelectedNode(t *testing.T) {
	tree := base.NewTree(files()...)
	tree.Select(1)
	var observed []string
	tree.Subscribe(func() {
		current, _ := tree.Current()
		observed = append(observed, current)
	})
	for _, change := range []func() bool{
		func() bool { return tree.Open(0) },
		func() bool { return tree.Open(2) },
		func() bool { return tree.Close(0) },
	} {
		if !change() {
			t.Fatal("branch did not change")
		}
		if got, _ := tree.Current(); got != "README" {
			t.Fatalf("changing another branch moved selection to %q", got)
		}
	}
	if len(observed) != 3 {
		t.Fatalf("published %d changes, want one complete change per branch transition", len(observed))
	}
	for _, got := range observed {
		if got != "README" {
			t.Fatalf("subscriber saw a partial selection on %q", got)
		}
	}
}

func TestTreeCollapseSelectsTheAncestorOfAHiddenNode(t *testing.T) {
	tree := base.NewTree(files()...)
	tree.Open(0)
	tree.Open(2)
	tree.Select(3)
	if !tree.Close(0) {
		t.Fatal("branch did not close")
	}
	if got, _ := tree.Current(); got != "core" {
		t.Fatalf("hiding the selected descendant moved selection to %q, want its ancestor", got)
	}
}

func TestTreeBranchChangesCancelDeferredNavigation(t *testing.T) {
	for _, test := range []struct {
		name string
		open bool
	}{{"open", true}, {"close", false}} {
		t.Run(test.name, func(t *testing.T) {
			keys, resolve := deferredKeys(base.SelectNext, base.SelectLast)
			tree := base.NewTree(files()...)
			tree.Keys = keys
			if !test.open {
				tree.Open(0)
			}
			root := newTestRoot(tree.Element())
			t.Cleanup(root.root.Close)
			focusElement(t, tree.Element())
			if !root.root.Handle(input.Key{Rune: 'x'}) || *resolve == nil {
				t.Fatal("tree navigation was not deferred")
			}
			if test.open {
				tree.Open(0)
			} else {
				tree.Close(0)
			}
			(*resolve)()
			if got, _ := tree.Current(); got != "core" {
				t.Fatalf("obsolete navigation selected %q", got)
			}
		})
	}
}
