package input_test

import (
	"testing"

	"github.com/Tangerg/oolong/core/input"
)

func TestMouseReleaseMatchesItsCapturedButton(t *testing.T) {
	buttons := [...]input.Button{input.ButtonLeft, input.ButtonMiddle, input.ButtonRight}
	for _, test := range []struct {
		name  string
		event input.Mouse
		want  [3]bool
	}{
		{"unspecified release", input.Mouse{Action: input.MouseUp}, [3]bool{true, true, true}},
		{"left release", input.Mouse{Action: input.MouseUp, Button: input.ButtonLeft}, [3]bool{true, false, false}},
		{"middle release", input.Mouse{Action: input.MouseUp, Button: input.ButtonMiddle}, [3]bool{false, true, false}},
		{"right release", input.Mouse{Action: input.MouseUp, Button: input.ButtonRight}, [3]bool{false, false, true}},
		{"press", input.Mouse{Action: input.MouseDown, Button: input.ButtonLeft}, [3]bool{}},
		{"drag", input.Mouse{Action: input.MouseDrag, Button: input.ButtonLeft}, [3]bool{}},
		{"move", input.Mouse{Action: input.MouseMove}, [3]bool{}},
		{"wheel up", input.Mouse{Action: input.WheelUp}, [3]bool{}},
		{"wheel down", input.Mouse{Action: input.WheelDown}, [3]bool{}},
	} {
		t.Run(test.name, func(t *testing.T) {
			for i, captured := range buttons {
				if got := test.event.Releases(captured); got != test.want[i] {
					t.Fatalf("Releases(%v) = %v, want %v", captured, got, test.want[i])
				}
			}
		})
	}
}
