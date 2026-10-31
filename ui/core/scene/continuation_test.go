package scene_test

import (
	"errors"
	"testing"

	"github.com/Tangerg/oolong/ui/core/scene"
)

func TestContinuationBelongsToItsSemanticTraversal(t *testing.T) {
	makeRoot := func() *scene.Root {
		group := func(key string) scene.Element {
			g := scene.NewGroup(scene.GroupConfig{Element: scene.ElementConfig{Key: key}, Items: []scene.Item{item("leaf", scene.NewElement(scene.ElementConfig{Key: "leaf"}))}})
			return g.Element()
		}
		return root(t, scene.NewGroup(scene.GroupConfig{Items: []scene.Item{item("a", group("a")), item("b", group("b"))}}).Element())
	}
	a, b := makeRoot(), makeRoot()
	page, err := a.Observe(scene.Query{RootKey: "a", Limit: 1})
	if err != nil || len(page.Nodes) != 1 || !page.Truncated {
		t.Fatal(page, err)
	}
	for _, c := range []struct {
		r *scene.Root
		q scene.Query
	}{{b, scene.Query{RootKey: "a"}}, {a, scene.Query{RootKey: "b"}}, {a, scene.Query{RootKey: "a", Depth: 1}}} {
		c.q.Cursor = page.Next
		if _, err := c.r.Observe(c.q); !errors.Is(err, scene.ErrObservationChanged) {
			t.Errorf("mixed traversal accepted: %v", err)
		}
	}
	continued, err := a.Observe(scene.Query{Root: page.Nodes[0].Target, Limit: 8, Cursor: page.Next})
	if err != nil || len(continued.Nodes) != 1 || continued.Nodes[0].Key != "leaf" {
		t.Fatal(continued, err)
	}
}
