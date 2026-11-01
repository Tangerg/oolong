package scene_test

import (
	"context"
	"testing"

	"github.com/Tangerg/oolong/ui/core/scene"
)

func TestErasedNullableArgumentHasTheSameDomainAsItsTypedAction(t *testing.T) {
	var calls, checks int
	action := scene.NewAction(scene.ActionConfig[*int, int]{ID: scene.ActionSetValue, Availability: func(*int) scene.Availability { checks++; return scene.Availability{} }, Run: func(_ context.Context, p *int) (scene.ActionResult[int], error) {
		if p != nil {
			t.Fatal("expected nil")
		}
		calls++
		return scene.ActionResult[int]{Value: calls}, nil
	}})
	e := scene.NewElement(scene.ElementConfig{Actions: []scene.ActionBinding{action.Binding()}})
	r := root(t, e)
	o, err := r.Observe(scene.Query{})
	if err != nil || len(o.Nodes) != 1 || !o.Nodes[0].Actions[0].RequiresParameters || checks != 0 {
		t.Fatal("discovery invented an argument", o, err, checks)
	}
	if _, err = action.Invoke(t.Context(), nil); err != nil {
		t.Fatal(err)
	}
	v, err := r.Invoke(t.Context(), e.Target(), scene.ActionSetValue, nil)
	if err != nil || v.Value != 2 || checks != 2 {
		t.Fatalf("erased nil: %+v %v checks=%d", v, err, checks)
	}
}

func TestObservedRelationsNameLiveAttachmentsInTheObservedRoot(t *testing.T) {
	good := scene.NewElement(scene.ElementConfig{Key: "label"})
	gone := scene.NewElement(scene.ElementConfig{Key: "old"})
	foreign := scene.NewElement(scene.ElementConfig{Key: "elsewhere"})
	root(t, foreign)
	var old scene.Target
	e := scene.NewElement(scene.ElementConfig{Key: "field", Semantics: func() scene.Semantic {
		return scene.Semantic{Relations: []scene.Relation{{Target: foreign.Target()}, {Target: old}, {Target: good.Target()}}}
	}})
	g := scene.NewGroup(scene.GroupConfig{Items: []scene.Item{item("old", gone), item("label", good), item("field", e)}})
	r := root(t, g.Element())
	old = gone.Target()
	if err := g.Set(item("field", e), item("label", good)); err != nil {
		t.Fatal(err)
	}
	got, err := r.Observe(scene.Query{RootKey: "field"})
	if err != nil || len(got.Nodes) != 1 {
		t.Fatal(got, err)
	}
	links := got.Nodes[0].Relations
	if len(links) != 1 || links[0].Target != good.Target() {
		t.Fatalf("exposed dangling or foreign relation: %v", links)
	}
}
