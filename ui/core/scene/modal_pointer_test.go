package scene_test

import (
	"image"
	"slices"
	"testing"

	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/scene"
)

// A pointer a modal layer blocks used to reach only that layer's Handle: its
// Preview was skipped, and what it covered stayed hovered.
func TestABlockedPointerReachesTheModalThroughItsRoute(t *testing.T) {
	var covered []input.MouseAction
	var previews, handled int
	background := scene.NewElement(scene.ElementConfig{Handle: func(e scene.Event) scene.EventResult {
		covered = append(covered, e.Input().(input.Mouse).Action)
		return scene.EventResult{Consumed: true}
	}})
	modal := scene.NewGroup(scene.GroupConfig{Modal: true, Element: scene.ElementConfig{
		Preview: func(scene.Event) scene.EventResult { previews++; return scene.EventResult{} },
		Handle:  func(scene.Event) scene.EventResult { handled++; return scene.EventResult{Consumed: true} },
	}})
	g := scene.NewGroup(scene.GroupConfig{
		Overlay: true,
		Items:   []scene.Item{item("background", background), item("modal", modal.Element())},
		Layout: func(space image.Point, _ []scene.Item) []image.Rectangle {
			return []image.Rectangle{image.Rect(0, 0, space.X, space.Y), image.Rect(0, 0, 2, 1)}
		},
	})
	r := root(t, g.Element())
	draw(t, r, 6, 2, 0)
	r.Handle(input.Mouse{Action: input.MouseMove, Pos: image.Pt(4, 1)})
	if err := modal.Set(item("inside", scene.NewElement(scene.ElementConfig{Focusable: func() bool { return true }}))); err != nil {
		t.Fatal(err)
	}
	draw(t, r, 6, 2, 0)
	r.Handle(input.Mouse{Action: input.MouseDown, Button: input.ButtonLeft, Pos: image.Pt(4, 1)})
	if previews != 1 || handled != 1 {
		t.Fatalf("modal preview=%d handle=%d, want both once", previews, handled)
	}
	if !slices.Contains(covered, input.MouseLeave) || slices.Contains(covered, input.MouseDown) {
		t.Fatalf("covered element saw %v, want it left and not pressed", covered)
	}
}

// Capturing claims a gesture. A handler that asked to capture without also
// saying it consumed the press used to be ignored.
func TestCaptureImpliesConsumption(t *testing.T) {
	var events []input.MouseAction
	element := scene.NewElement(scene.ElementConfig{Handle: func(e scene.Event) scene.EventResult {
		events = append(events, e.Input().(input.Mouse).Action)
		return scene.EventResult{Capture: true}
	}})
	r := root(t, element)
	draw(t, r, 4, 1, 0)
	r.Handle(input.Mouse{Action: input.MouseDown, Button: input.ButtonLeft})
	r.Handle(input.Mouse{Action: input.MouseDrag, Button: input.ButtonLeft, Pos: image.Pt(9, 9)})
	if !slices.Equal(events, []input.MouseAction{input.MouseDown, input.MouseDrag}) {
		t.Fatalf("events = %v, want the drag outside delivered to the capture", events)
	}
}
