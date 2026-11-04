package scene_test

import (
	"slices"
	"testing"

	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/scene"
)

func TestGestureEndsWhenReleaseValidationUnwinds(t *testing.T) {
	for _, phase := range []string{"eligibility", "content"} {
		t.Run(phase, func(t *testing.T) {
			interrupt := false
			var events []input.MouseAction
			control := scene.NewElement(scene.ElementConfig{
				Focusable: func() bool { return true },
				Enabled: func() bool {
					if interrupt && phase == "eligibility" {
						panic("policy failed")
					}
					return true
				},
				Semantics: func() scene.Semantic {
					if interrupt && phase == "content" {
						panic("policy failed")
					}
					return scene.Semantic{ContentID: "item"}
				},
				Handle: func(e scene.Event) scene.EventResult {
					mouse := e.Input().(input.Mouse)
					events = append(events, mouse.Action)
					return scene.EventResult{Consumed: true, Capture: mouse.Action == input.MouseDown}
				},
			})
			r := root(t, control)
			draw(t, r, 2, 1, 0)
			r.Handle(input.Mouse{Action: input.MouseDown, Button: input.ButtonLeft})
			interrupt = true
			func() {
				defer func() {
					if failure := recover(); failure != "policy failed" {
						t.Errorf("validation failure = %v", failure)
					}
				}()
				r.Handle(input.Mouse{Action: input.MouseUp, Button: input.ButtonLeft})
			}()
			interrupt = false
			r.Handle(input.Mouse{Action: input.MouseUp, Button: input.ButtonLeft})
			want := []input.MouseAction{input.MouseDown, input.MouseCancel}
			if !slices.Equal(events, want) {
				t.Fatalf("interrupted release retained a live gesture: events=%v, want %v", events, want)
			}
		})
	}
}

func TestSupersedingPressEndsTheOldGestureBeforePolicyFailureEscapes(t *testing.T) {
	interrupt := false
	var events []input.MouseAction
	control := scene.NewElement(scene.ElementConfig{
		Focusable: func() bool {
			if interrupt {
				panic("focus policy failed")
			}
			return true
		},
		Handle: func(e scene.Event) scene.EventResult {
			mouse := e.Input().(input.Mouse)
			events = append(events, mouse.Action)
			return scene.EventResult{Consumed: true, Capture: mouse.Action == input.MouseDown}
		},
	})
	r := root(t, control)
	draw(t, r, 2, 1, 0)
	press := input.Mouse{Action: input.MouseDown, Button: input.ButtonLeft}
	r.Handle(press)
	interrupt = true
	func() {
		defer func() {
			if failure := recover(); failure != "focus policy failed" {
				t.Errorf("validation failure = %v", failure)
			}
		}()
		r.Handle(press)
	}()
	interrupt = false
	r.Handle(input.Mouse{Action: input.MouseUp, Button: input.ButtonLeft})
	want := []input.MouseAction{input.MouseDown, input.MouseCancel}
	if !slices.Equal(events, want) {
		t.Fatalf("interrupted replacement press retained the old gesture: events=%v, want %v", events, want)
	}
}
