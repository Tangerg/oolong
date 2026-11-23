package base_test

import (
	"image"
	"slices"
	"testing"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/internal/scenetest"
)

func TestListActivationFollowsTheKeyAndAClickOnOneRow(t *testing.T) {
	scenes := scenetest.New()
	t.Cleanup(scenes.Close)
	var activated []string
	list := base.NewList(base.ListConfig[string]{
		Items:    []string{"one", "two", "three"},
		Activate: func(_ int, item string) { activated = append(activated, item) },
	})
	root := scenes.Mount(list)
	root.Draw(grid.NewSurface(10, 3).View())
	scenes.Focus(list, true)
	press := func(action input.MouseAction, row int) {
		root.Handle(input.Mouse{Pos: image.Pt(1, row), Action: action, Button: input.ButtonLeft})
	}

	root.Handle(input.Key{Code: input.Down})
	root.Handle(input.Key{Code: input.Enter})
	press(input.MouseDown, 2)
	press(input.MouseUp, 2)
	press(input.MouseDown, 0)
	press(input.MouseDrag, 1)
	press(input.MouseUp, 1)
	if want := []string{"two", "three"}; !slices.Equal(activated, want) {
		t.Fatalf("activated %q, want %q: a drag to another row must not activate", activated, want)
	}
	if list.Selected() != 1 {
		t.Fatalf("drag left the cursor on %d, want 1", list.Selected())
	}
}

func TestAListWithoutActivationLeavesTheKeyToItsContainer(t *testing.T) {
	scenes := scenetest.New()
	t.Cleanup(scenes.Close)
	list := base.NewList(base.ListConfig[string]{Items: []string{"one"}})
	scenes.Mount(list)
	if list.Handle(input.Key{Code: input.Enter}) {
		t.Fatal("a list with no activation consumed the activation key")
	}
}

func TestListActivationIsASemanticAction(t *testing.T) {
	scenes := scenetest.New()
	t.Cleanup(scenes.Close)
	var activated []int
	list := base.NewList(base.ListConfig[string]{
		Items:    []string{"one", "two"},
		Activate: func(at int, _ string) { activated = append(activated, at) },
	})
	root := scenes.Mount(list)
	observed, err := root.Observe(scene.Query{})
	if err != nil {
		t.Fatal(err)
	}
	address := base.CollectionIndex{ContentRevision: observed.Nodes[0].ContentRevision, Index: 1}
	result, err := root.Invoke(t.Context(), list.Element().Target(), scene.ActionActivate, address)
	if err != nil || result.State != scene.Applied || list.Selected() != 1 || !slices.Equal(activated, []int{1}) {
		t.Fatalf("activate = %+v, %v; cursor %d; activated %v", result, err, list.Selected(), activated)
	}
}
