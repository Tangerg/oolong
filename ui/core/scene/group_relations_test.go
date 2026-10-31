package scene_test

import (
	"errors"
	"testing"

	"github.com/Tangerg/oolong/ui/core/scene"
)

func TestSiblingRelationsRejectTheWholeChange(t *testing.T) {
	left, right, foreign := scene.NewGroup(scene.GroupConfig{}), scene.NewGroup(scene.GroupConfig{}), scene.NewGroup(scene.GroupConfig{})
	parent := scene.NewGroup(scene.GroupConfig{Items: []scene.Item{{Element: left.Element()}, {Element: right.Element()}}})
	r := root(t, parent.Element())
	_ = root(t, foreign.Element())
	original := scene.NewElement(scene.ElementConfig{})
	if err := left.Set(scene.Item{Element: original}); err != nil {
		t.Fatal(err)
	}
	lease := original.Target()
	shared := scene.NewElement(scene.ElementConfig{})
	change := func(g *scene.Group, e scene.Element) scene.GroupItems {
		return scene.GroupItems{Group: g, Items: []scene.Item{{Element: e}}}
	}
	for _, tc := range []struct {
		name   string
		second scene.GroupItems
		want   error
	}{
		{"shared node", change(right, shared), scene.ErrDuplicate},
		{"repeated group", change(left, shared), scene.ErrDuplicate},
		{"late cycle", change(right, parent.Element()), scene.ErrCycle},
		{"foreign root", change(foreign, shared), scene.ErrInvalid},
		{"cross-parent transfer", change(right, original), scene.ErrMounted},
	} {
		t.Run(tc.name, func(t *testing.T) {
			changes := []scene.GroupItems{change(left, shared), tc.second}
			if err := parent.CheckChildItems(changes...); !errors.Is(err, tc.want) {
				t.Fatalf("preflight=%v want %v", err, tc.want)
			}
			wake := r.Changes()
			if err := parent.SetChildItems(changes...); !errors.Is(err, tc.want) {
				t.Fatalf("set=%v want %v", err, tc.want)
			}
			if original.Target() != lease || shared.Alive() || len(right.Items()) != 0 {
				t.Fatal("invalid sibling update altered an attachment")
			}
			select {
			case <-wake:
				t.Fatal("invalid sibling update woke the scene")
			default:
			}
		})
	}
}
