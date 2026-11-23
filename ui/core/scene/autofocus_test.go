package scene_test

import (
	"testing"

	"github.com/Tangerg/oolong/ui/core/scene"
)

func focusable() scene.Element {
	return scene.NewElement(scene.ElementConfig{Focusable: func() bool { return true }})
}

// An input below a log starts with the keys without the log giving up being
// focusable.
func TestFocusStartsInsideAnAutofocusElement(t *testing.T) {
	transcript, editor := focusable(), focusable()
	input := scene.NewGroup(scene.GroupConfig{Element: scene.ElementConfig{Autofocus: true}, Items: []scene.Item{item("editor", editor)}})
	preview, field := focusable(), focusable()
	decision := scene.NewGroup(scene.GroupConfig{Items: []scene.Item{item("field", field)}})
	decision.Element().SetAutofocus(true)
	layer := scene.NewGroup(scene.GroupConfig{Modal: true})
	r := root(t, scene.NewGroup(scene.GroupConfig{Overlay: true, Items: []scene.Item{
		item("transcript", transcript), item("input", input.Element()), item("layer", layer.Element()),
	}}).Element())

	r.Changed()
	if !editor.Focused() {
		t.Fatal("focus started outside the autofocus element")
	}
	if err := transcript.RequestFocus(); err != nil {
		t.Fatalf("the element before the autofocus element stopped being focusable: %v", err)
	}

	if err := layer.Set(item("preview", preview), item("decision", decision.Element())); err != nil {
		t.Fatal(err)
	}
	r.Changed()
	if !field.Focused() {
		t.Fatal("an opened layer did not start focus in its autofocus element")
	}
	if err := layer.Set(); err != nil {
		t.Fatal(err)
	}
	r.Changed()
	if !transcript.Focused() {
		t.Fatal("closing the layer did not return focus to its opener")
	}
}

func TestFocusFallsBackToTreeOrderWithoutAnAutofocusTarget(t *testing.T) {
	first := focusable()
	empty := scene.NewGroup(scene.GroupConfig{Element: scene.ElementConfig{Autofocus: true}})
	r := root(t, scene.NewGroup(scene.GroupConfig{Items: []scene.Item{item("first", first), item("empty", empty.Element())}}).Element())
	r.Changed()
	if !first.Focused() {
		t.Fatal("an autofocus element with nothing focusable kept focus from the first focusable element")
	}
}
