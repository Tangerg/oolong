package scene_test

import (
	"slices"
	"testing"

	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/scene"
)

func counter(count *int) scene.Element {
	return scene.NewElement(scene.ElementConfig{Focusable: func() bool { return true }, Handle: func(scene.Event) scene.EventResult {
		*count++
		return scene.EventResult{Consumed: true}
	}})
}

// A key typed while the screen still shows what lies beneath a layer was meant
// for that, not for an answer the user has not seen.
func TestAModalTakesKeysOnlyOnceItReachesTheScreen(t *testing.T) {
	var below, first, second int
	layer := scene.NewGroup(scene.GroupConfig{Modal: true})
	r := root(t, scene.NewGroup(scene.GroupConfig{Overlay: true, Items: []scene.Item{item("below", counter(&below)), item("layer", layer.Element())}}).Element())
	draw(t, r, 4, 1, 0)

	body := counter(&first)
	if err := layer.Set(item("body", body)); err != nil {
		t.Fatal(err)
	}
	if !r.Handle(input.Key{Code: input.Enter}) || !r.Handle(input.Paste{Text: "yes"}) || first != 0 || below != 0 {
		t.Fatalf("unseen layer: consumed answer=%d below=%d", first, below)
	}
	r.Handle(input.FocusIn{})
	if first != 1 {
		t.Fatalf("a focus notice waited for the layer: answer=%d", first)
	}
	first = 0
	draw(t, r, 4, 1, 0)
	r.Handle(input.Key{Code: input.Enter})
	if first != 1 {
		t.Fatalf("presented layer answer=%d", first)
	}

	r.InvalidateGeometry()
	r.Handle(input.Key{Code: input.Enter})
	if first != 2 {
		t.Fatalf("a resize revoked what was already seen: answer=%d", first)
	}

	if err := layer.Set(item("replacement", counter(&second))); err != nil {
		t.Fatal(err)
	}
	r.Handle(input.Key{Code: input.Enter})
	if second != 0 || first != 2 {
		t.Fatalf("replaced content answered before it was shown: first=%d second=%d", first, second)
	}
	draw(t, r, 4, 1, 0)
	r.Handle(input.Key{Code: input.Enter})
	if second != 1 {
		t.Fatalf("shown replacement answer=%d", second)
	}

	if err := layer.Set(); err != nil {
		t.Fatal(err)
	}
	draw(t, r, 4, 1, 0)
	if err := layer.Set(item("body", body)); err != nil {
		t.Fatal(err)
	}
	r.Handle(input.Key{Code: input.Enter})
	if first != 2 || below != 0 {
		t.Fatalf("a reopened layer kept its earlier presentation: answer=%d below=%d", first, below)
	}
}

func TestShortcutsReachAroundAModalLayer(t *testing.T) {
	var order []string
	var previewed, answered int
	quit := input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl}
	shortcut := func(name string) func(scene.Event) scene.EventResult {
		return func(event scene.Event) scene.EventResult {
			order = append(order, name)
			key, _ := event.Input().(input.Key)
			return scene.EventResult{Consumed: name == "layers" && key.IsRune(quit.Rune, quit.Mods)}
		}
	}
	layer := scene.NewGroup(scene.GroupConfig{Modal: true, Items: []scene.Item{item("body", counter(&answered))}})
	layers := scene.NewGroup(scene.GroupConfig{Overlay: true, Element: scene.ElementConfig{Shortcut: shortcut("layers")}, Items: []scene.Item{item("layer", layer.Element())}})
	r := root(t, scene.NewGroup(scene.GroupConfig{Element: scene.ElementConfig{Shortcut: shortcut("application"), Preview: func(scene.Event) scene.EventResult {
		previewed++
		return scene.EventResult{Consumed: true}
	}}, Items: []scene.Item{item("layers", layers.Element())}}).Element())
	draw(t, r, 4, 1, 0)

	r.Handle(input.Key{Code: input.Enter})
	if !slices.Equal(order, []string{"application", "layers"}) || answered != 1 || previewed != 0 {
		t.Fatalf("passing key: order=%v answered=%d previewed=%d", order, answered, previewed)
	}

	order = nil
	r.Handle(quit)
	if !slices.Equal(order, []string{"application", "layers"}) || answered != 1 {
		t.Fatalf("consumed shortcut: order=%v answered=%d", order, answered)
	}

	order = nil
	if err := layer.Set(item("replacement", counter(&answered))); err != nil {
		t.Fatal(err)
	}
	r.Handle(quit)
	if len(order) != 2 {
		t.Fatalf("a shortcut waited for the layer it does not answer: %v", order)
	}

	order = nil
	r.Handle(input.Paste{Text: "x"})
	if len(order) != 0 {
		t.Fatalf("a shortcut saw a paste: %v", order)
	}
}
