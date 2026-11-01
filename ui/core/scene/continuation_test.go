package scene_test

import (
	"errors"
	"strconv"
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
		if _, pageErr := c.r.Observe(c.q); !errors.Is(pageErr, scene.ErrObservationChanged) {
			t.Errorf("mixed traversal accepted: %v", pageErr)
		}
	}
	continued, err := a.Observe(scene.Query{Root: page.Nodes[0].Target, Limit: 8, Cursor: page.Next})
	if err != nil || len(continued.Nodes) != 1 || continued.Nodes[0].Key != "leaf" {
		t.Fatal(continued, err)
	}
}

func TestContinuationResumesWithoutReplayingEarlierSiblings(t *testing.T) {
	const count = 128
	items := make([]scene.Item, count)
	var reads int
	for i := range items {
		items[i].Element = scene.NewElement(scene.ElementConfig{Semantics: func() scene.Semantic {
			reads++
			return scene.Semantic{Name: strconv.Itoa(i)}
		}})
	}
	r := root(t, scene.NewGroup(scene.GroupConfig{Items: items}).Element())
	query := scene.Query{Limit: 4}
	for seen := 0; ; {
		page, err := r.Observe(query)
		if err != nil {
			t.Fatal(err)
		}
		for _, n := range page.Nodes {
			if n.Name != strconv.Itoa(seen) {
				t.Fatalf("node %d named %q", seen, n.Name)
			}
			seen++
		}
		query.Cursor = page.Next
		if page.Next == (scene.Cursor{}) {
			if seen != count {
				t.Fatalf("observed %d nodes", seen)
			}
			break
		}
	}
	if reads > count*2 {
		t.Fatalf("pagination projected %d siblings %d times", count, reads)
	}
}
