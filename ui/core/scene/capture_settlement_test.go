package scene_test

import (
	"slices"
	"testing"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/scene"
)

// A notification flushed by a release can prepare a frame. Ending the gesture
// then ran the captured handler while that candidate still awaited settlement,
// when the model must not change.
func TestEndingAGestureWaitsForAPreparedFrame(t *testing.T) {
	var r *scene.Root
	var prepared *scene.Prepared
	var events []input.MouseAction
	pendingAtCancel := false
	captured := scene.NewElement(scene.ElementConfig{
		Focusable: func() bool { return true },
		Handle: func(e scene.Event) scene.EventResult {
			mouse := e.Input().(input.Mouse)
			events = append(events, mouse.Action)
			if mouse.Action == input.MouseCancel {
				pendingAtCancel = prepared != nil
			}
			return scene.EventResult{Consumed: true, Capture: mouse.Action == input.MouseDown}
		},
	})
	other := scene.NewElement(scene.ElementConfig{
		Focusable: func() bool { return true },
		FocusChanged: func(focused bool) {
			if !focused || prepared != nil {
				return
			}
			p, err := r.Prepare(grid.NewSurface(2, 2).View())
			if err != nil {
				t.Error(err)
				return
			}
			prepared = p
		},
	})
	r = root(t, scene.NewGroup(scene.GroupConfig{Items: []scene.Item{item("captured", captured), item("other", other)}}).Element())
	draw(t, r, 2, 2, 0)
	r.Handle(input.Mouse{Action: input.MouseDown, Button: input.ButtonLeft})
	if err := other.RequestFocus(); err != nil {
		t.Fatal(err)
	}
	r.Handle(input.Mouse{Action: input.MouseUp, Button: input.ButtonLeft})
	if prepared == nil {
		t.Fatal("the fixture did not prepare a frame during the release")
	}
	if slices.Contains(events, input.MouseCancel) {
		t.Fatalf("the gesture ended while a frame was pending: %v", events)
	}
	prepared.Abort()
	prepared = nil
	r.FlushNotifications()
	if !slices.Contains(events, input.MouseCancel) || pendingAtCancel {
		t.Fatalf("the gesture did not end after the frame settled: events=%v pending=%v", events, pendingAtCancel)
	}
}
