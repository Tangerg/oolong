package headless_test

import (
	"image"
	"testing"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
)

func TestSliderOwnsOneBoundedValue(t *testing.T) {
	slider := headless.NewSlider(-10, 10)
	if got := slider.Value(); got != -10 {
		t.Fatalf("initial value = %d, want minimum -10", got)
	}
	if !slider.Set(3) || slider.Value() != 3 {
		t.Fatalf("Set(3) left value %d", slider.Value())
	}
	if !slider.Set(99) || slider.Value() != 10 {
		t.Fatalf("Set past maximum left value %d", slider.Value())
	}
	if slider.Set(10) {
		t.Fatal("setting the current value reported a change")
	}

	slider.SetStep(4)
	slider.Move(-2)
	if got := slider.Value(); got != 2 {
		t.Fatalf("two decreases left %d, want 2", got)
	}
	slider.Move(-100)
	if got := slider.Value(); got != -10 {
		t.Fatalf("large decrease left %d, want minimum", got)
	}
	slider.Move(100)
	if got := slider.Value(); got != 10 {
		t.Fatalf("large increase left %d, want maximum", got)
	}
}

func TestControlledSliderHasNoShadowValue(t *testing.T) {
	value := 50
	slider := headless.NewControlledSlider(headless.Bind(&value), 0, 10)
	if value != 10 || slider.Value() != 10 {
		t.Fatalf("construction left binding=%d slider=%d, want both clamped to 10", value, slider.Value())
	}

	value = -4
	if got := slider.Value(); got != 0 {
		t.Fatalf("owner-written value reads as %d, want clamped 0", got)
	}
	if value != -4 {
		t.Fatal("a read silently rewrote controlled state")
	}
	if !slider.Sync() || value != 0 {
		t.Fatalf("Sync left binding %d, want 0", value)
	}
	slider.Set(7)
	if value != 7 {
		t.Fatalf("Set wrote %d to binding, want 7", value)
	}
}

func TestSliderAnswersActionsAndDefaultKeys(t *testing.T) {
	slider := headless.NewSlider(0, 10)
	slider.SetStep(2)
	for _, key := range []input.Key{
		{Code: input.Right},
		{Code: input.Up},
	} {
		if !slider.Handle(key) {
			t.Fatalf("slider declined %v", key)
		}
	}
	if got := slider.Value(); got != 4 {
		t.Fatalf("two increases left %d, want 4", got)
	}
	if !slider.Do(headless.ToMaximum) || slider.Value() != 10 {
		t.Fatalf("ToMaximum left %d", slider.Value())
	}
	if !slider.Handle(input.Key{Code: input.Home}) || slider.Value() != 0 {
		t.Fatalf("Home left %d", slider.Value())
	}
	if slider.Handle(input.Key{Code: input.Enter}) {
		t.Fatal("slider consumed an unrelated key")
	}
}

type sliderTrack struct {
	control *headless.Slider
	rect    image.Rectangle
}

func (s sliderTrack) Draw(frame headless.Frame) { s.control.Stage(frame, s.rect) }

func TestSliderDragsAgainstCommittedTrackGeometry(t *testing.T) {
	slider := headless.NewSlider(0, 100)
	root := headless.NewRoot(sliderTrack{control: slider, rect: grid.Rect(2, 0, 5, 1)})
	root.Draw(grid.NewSurface(10, 1).View())

	if slider.Handle(input.Mouse{Pos: image.Pt(0, 0), Action: input.MouseDown, Button: input.ButtonLeft}) {
		t.Fatal("slider accepted a press outside its committed track")
	}
	if !slider.Handle(input.Mouse{Pos: image.Pt(4, 0), Action: input.MouseDown, Button: input.ButtonLeft}) {
		t.Fatal("slider declined a press on its track")
	}
	if got := slider.Value(); got != 50 {
		t.Fatalf("middle press chose %d, want 50", got)
	}
	if !slider.Handle(input.Mouse{Pos: image.Pt(50, 0), Action: input.MouseDrag, Button: input.ButtonLeft}) || slider.Value() != 100 {
		t.Fatalf("drag past end left %d, want 100", slider.Value())
	}
	if !slider.Handle(input.Mouse{Pos: image.Pt(-5, 0), Action: input.MouseUp, Button: input.ButtonLeft}) || slider.Value() != 0 {
		t.Fatalf("release past start left %d, want 0", slider.Value())
	}
	if slider.Handle(input.Mouse{Pos: image.Pt(5, 0), Action: input.MouseDrag, Button: input.ButtonLeft}) {
		t.Fatal("slider kept a drag after release")
	}
}

func TestSliderSemanticsCarryItsMeaning(t *testing.T) {
	slider := headless.NewSlider(0, 10)
	slider.SetLabel("speed")
	slider.Set(4)
	slider.Focus(true)
	node := slider.Semantics()
	if node.Role != headless.RoleSlider || node.Label != "speed" || node.Value != "4" || !node.State.Has(headless.StateFocused) {
		t.Fatalf("semantics = %+v", node)
	}
}

func TestSliderRejectsInvalidConfiguration(t *testing.T) {
	for _, build := range []func(){
		func() { headless.NewSlider(2, 1) },
		func() {
			slider := headless.NewSlider(0, 1)
			slider.SetStep(0)
		},
		func() {
			maxInt := int(^uint(0) >> 1)
			headless.NewSlider(-maxInt-1, maxInt)
		},
	} {
		func() {
			defer func() {
				if recover() == nil {
					t.Error("invalid slider configuration did not panic")
				}
			}()
			build()
		}()
	}
}
