package scene_test

import (
	"image"
	"slices"
	"testing"

	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/scene"
)

// A handler that only knows whether it consumed input still owns the gesture it
// started: the press focuses it and the drag outside comes back to it.
func TestAConsumedLeftPressFocusesAndCapturesThroughResult(t *testing.T) {
	var events []input.MouseAction
	target := scene.NewElement(scene.ElementConfig{
		Focusable: func() bool { return true },
		Handle: func(e scene.Event) scene.EventResult {
			mouse, ok := e.Input().(input.Mouse)
			if ok {
				events = append(events, mouse.Action)
			}
			return e.Result(ok)
		},
	})
	other := scene.NewElement(scene.ElementConfig{Focusable: func() bool { return true }})
	g := scene.NewGroup(scene.GroupConfig{Items: []scene.Item{item("target", target), item("other", other)}})
	r := root(t, g.Element())
	if err := other.RequestFocus(); err != nil {
		t.Fatal(err)
	}
	draw(t, r, 10, 2, 0)
	r.Handle(input.Mouse{Action: input.MouseDown, Button: input.ButtonLeft})
	r.Handle(input.Mouse{Action: input.MouseDrag, Button: input.ButtonLeft, Pos: image.Pt(30, 30)})
	r.Handle(input.Mouse{Action: input.MouseUp, Button: input.ButtonLeft, Pos: image.Pt(30, 30)})
	if !slices.Equal(events, []input.MouseAction{input.MouseDown, input.MouseDrag, input.MouseUp}) {
		t.Fatalf("events = %v, want the whole gesture delivered to the press target", events)
	}
	if !target.Focused() {
		t.Fatal("a consumed left press did not take focus")
	}
}

func TestResultClaimsNothingBeyondConsumptionForOtherInput(t *testing.T) {
	var answers []scene.EventResult
	target := scene.NewElement(scene.ElementConfig{Handle: func(e scene.Event) scene.EventResult {
		mouse, _ := e.Input().(input.Mouse)
		answer := e.Result(mouse.Button != input.ButtonNone || mouse.Action != input.MouseDown)
		answers = append(answers, answer)
		return answer
	}})
	r := root(t, target)
	draw(t, r, 4, 1, 0)
	r.Handle(input.Mouse{Action: input.MouseDown, Button: input.ButtonRight})
	r.Handle(input.Mouse{Action: input.WheelDown})
	want := []scene.EventResult{{Consumed: true}, {Consumed: true}}
	if !slices.Equal(answers, want) {
		t.Fatalf("answers = %+v, want %+v", answers, want)
	}
}
