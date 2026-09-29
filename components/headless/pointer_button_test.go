package headless_test

import (
	"image"
	"testing"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
)

func pointerReport(t *testing.T, parser *input.Parser, report string) input.Mouse {
	t.Helper()
	events := parser.Feed([]byte(report))
	if len(events) != 1 {
		t.Fatalf("decoded %d events from %q, want one", len(events), report)
	}
	mouse, ok := events[0].(input.Mouse)
	if !ok {
		t.Fatalf("decoded %T, want Mouse", events[0])
	}
	return mouse
}

func TestPointerReleaseMatchesCapturedButtonSGR(t *testing.T) {
	for _, test := range []struct {
		name    string
		presses []string
		foreign string
		release string
		button  input.Button
	}{
		{"left then right release", []string{"\x1b[<0;2;1M"}, "\x1b[<2;2;1m", "\x1b[<0;2;1m", input.ButtonLeft},
		{"right then left release", []string{"\x1b[<2;2;1M"}, "\x1b[<0;2;1m", "\x1b[<2;2;1m", input.ButtonRight},
		{"middle then left release", []string{"\x1b[<1;2;1M"}, "\x1b[<0;2;1m", "\x1b[<1;2;1m", input.ButtonMiddle},
		{"right supersedes left", []string{"\x1b[<0;2;1M", "\x1b[<2;2;1M"}, "\x1b[<0;2;1m", "\x1b[<2;2;1m", input.ButtonRight},
	} {
		t.Run(test.name, func(t *testing.T) {
			var pointer headless.Pointer
			var parser input.Parser
			stagePointer(&pointer, image.Rect(0, 0, 4, 1))
			for _, report := range test.presses {
				if !pointer.Handle(pointerReport(t, &parser, report)) {
					t.Fatal("press was declined")
				}
			}
			if pointer.Handle(pointerReport(t, &parser, test.foreign)) {
				t.Error("another button's release was consumed")
			}
			if !pointer.Pressing() || pointer.Clicked(test.button) {
				t.Fatal("another button's release ended the captured press")
			}
			if !pointer.Handle(pointerReport(t, &parser, test.release)) {
				t.Fatal("captured button's release was declined")
			}
			if pointer.Pressing() || !pointer.Clicked(test.button) || pointer.Clicked(test.button) {
				t.Fatal("matching release must end capture and complete exactly one click")
			}
		})
	}
}

func TestPointerUnspecifiedReleaseCompletesCapturedButton(t *testing.T) {
	for _, test := range []struct {
		press  string
		button input.Button
	}{
		{"\x1b[<0;2;1M", input.ButtonLeft},
		{"\x1b[<1;2;1M", input.ButtonMiddle},
		{"\x1b[<2;2;1M", input.ButtonRight},
	} {
		var pointer headless.Pointer
		var parser input.Parser
		stagePointer(&pointer, image.Rect(0, 0, 4, 1))
		pointer.Handle(pointerReport(t, &parser, test.press))
		release := pointerReport(t, &parser, "\x1b[<3;2;1m")
		if release.Button != input.ButtonNone || release.Action != input.MouseUp {
			t.Fatalf("unspecified release decoded as %+v", release)
		}
		if !pointer.Handle(release) || pointer.Pressing() || !pointer.Clicked(test.button) {
			t.Fatalf("unspecified release did not complete button %v", test.button)
		}
	}
}

func TestPointerMatchingReleaseCannotReviveEndedCapture(t *testing.T) {
	for _, end := range []string{"outside", "presentation gap", "left interface"} {
		t.Run(end, func(t *testing.T) {
			var pointer headless.Pointer
			var parser input.Parser
			stagePointer(&pointer, image.Rect(0, 0, 4, 1))
			pointer.Handle(pointerReport(t, &parser, "\x1b[<0;2;1M"))
			pointer.Handle(pointerReport(t, &parser, "\x1b[<2;2;1m"))
			release := "\x1b[<0;2;1m"
			switch end {
			case "outside":
				release = "\x1b[<0;9;1m"
			case "presentation gap":
				stagePointer(&pointer, image.Rectangle{})
				stagePointer(&pointer, image.Rect(0, 0, 4, 1))
			case "left interface":
				pointer.Left()
			}
			pointer.Handle(pointerReport(t, &parser, release))
			if pointer.Pressing() || pointer.Clicked(input.ButtonLeft) {
				t.Fatal("release revived an ended capture or clicked outside")
			}
		})
	}
}

func TestDialogTriggerWaitsForItsPressedButton(t *testing.T) {
	dialog := headless.NewDialog(headless.DialogConfig{Stack: &headless.Stack{}, Content: &panel{place: middle(8, 3)}})
	trigger := dialog.Trigger("Open", nil)
	paintWidget(12, 1, trigger)
	var parser input.Parser
	trigger.Handle(pointerReport(t, &parser, "\x1b[<0;2;1M"))
	trigger.Handle(pointerReport(t, &parser, "\x1b[<2;2;1m"))
	if dialog.Open() {
		t.Fatal("another button's release opened the dialog")
	}
	trigger.Handle(pointerReport(t, &parser, "\x1b[<0;2;1m"))
	if !dialog.Open() {
		t.Fatal("matching release did not open the dialog")
	}
}

func TestSliderIgnoresAnotherButtonsRelease(t *testing.T) {
	slider := headless.NewSlider(headless.SliderConfig{Maximum: 100})
	headless.NewRoot(sliderTrack{control: slider, rect: image.Rect(0, 0, 5, 1)}).
		Draw(grid.NewSurface(5, 1).View())
	var parser input.Parser
	slider.Handle(pointerReport(t, &parser, "\x1b[<0;3;1M"))
	if slider.Value() != 50 {
		t.Fatalf("middle press set value %d, want 50", slider.Value())
	}
	if slider.Handle(pointerReport(t, &parser, "\x1b[<2;5;1m")) || slider.Value() != 50 {
		t.Fatal("another button's release changed the slider")
	}
	if !slider.Handle(pointerReport(t, &parser, "\x1b[<0;5;1m")) || slider.Value() != 100 {
		t.Fatal("matching release did not finish the slider gesture")
	}
}
