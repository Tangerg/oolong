package scene_test

import (
	"testing"

	"github.com/Tangerg/oolong/ui/core/scene"
)

// A modal that stays mounted and opens by enabling itself was never seen by the
// root until its next settlement. Focusing inside it first recorded that inside
// control as the opener, so closing it lost the original focus.
func TestFocusingIntoAnEnabledModalKeepsItsOpener(t *testing.T) {
	first, opener, inside := control(), control(), control()
	open := false
	layer := scene.NewGroup(scene.GroupConfig{
		Modal:   true,
		Element: scene.ElementConfig{Enabled: func() bool { return open }},
		Items:   []scene.Item{item("inside", inside.element)},
	})
	g := scene.NewGroup(scene.GroupConfig{Items: []scene.Item{item("first", first.element), item("opener", opener.element), item("layer", layer.Element())}})
	r := root(t, g.Element())
	if err := opener.element.RequestFocus(); err != nil {
		t.Fatal(err)
	}
	open = true
	if err := inside.element.RequestFocus(); err != nil {
		t.Fatal(err)
	}
	layer.Element().Invalidate()
	open = false
	layer.Element().Invalidate()
	if r.Focus() != opener.element.Target() {
		t.Fatalf("closing the layer focused %v, want its opener", r.Focus())
	}
}
