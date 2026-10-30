package base_test

import (
	"image"
	"testing"

	"github.com/Tangerg/oolong/ui/core/scene"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
)

func TestSliderOwnsOneBoundedValue(t *testing.T) {
	slider := base.NewSlider(base.SliderConfig{Minimum: -10, Maximum: 10})
	if got := slider.Value(); got != 0 {
		t.Fatalf("initial value = %d, want configured zero", got)
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

func TestSliderClampsAtItsOnlyMutationBoundary(t *testing.T) {
	slider := base.NewSlider(base.SliderConfig{Initial: 50, Maximum: 10})
	if slider.Value() != 10 {
		t.Fatal("initial value was not clamped")
	}
	slider.Set(-4)
	if slider.Value() != 0 {
		t.Fatal("setter published out-of-range value")
	}
	slider.Set(7)
	slider.SetBounds(2, 5)
	if slider.Value() != 5 {
		t.Fatal("bounds did not atomically clamp current value")
	}
}

func TestSliderAnswersActionsAndDefaultKeys(t *testing.T) {
	slider := base.NewSlider(base.SliderConfig{Maximum: 10})
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
	if !slider.Do(base.ToMaximum) || slider.Value() != 10 {
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
	control *base.Slider
	rect    image.Rectangle
}

func (s sliderTrack) Draw(frame scene.Frame) { s.control.Stage(frame, s.rect) }

func TestSliderDragsAgainstCommittedTrackGeometry(t *testing.T) {
	slider := base.NewSlider(base.SliderConfig{Maximum: 100})
	root := newTestRoot(sliderTrack{control: slider, rect: grid.Area(2, 0, 5, 1)})
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
	slider := base.NewSlider(base.SliderConfig{Maximum: 10})
	slider.SetLabel("speed")
	slider.Set(4)
	testFocus(slider, true)
	observed, err := newTestRoot(slider).root.Observe(scene.Query{})
	if err != nil {
		t.Fatal(err)
	}
	if len(observed.Nodes) != 1 {
		t.Fatalf("semantic count=%d", len(observed.Nodes))
	}
	node := observed.Nodes[0]
	if node.Role != scene.RoleSlider || node.Name != "speed" || node.Value != "4" || node.State&scene.StateFocused == 0 {
		t.Fatalf("semantic=%+v", node)
	}
}

func TestSliderRejectsInvalidConfiguration(t *testing.T) {
	for _, build := range []func(){
		func() { base.NewSlider(base.SliderConfig{Minimum: 2, Maximum: 1}) },
		func() {
			slider := base.NewSlider(base.SliderConfig{Maximum: 1})
			slider.SetStep(0)
		},
		func() {
			maxInt := int(^uint(0) >> 1)
			base.NewSlider(base.SliderConfig{Minimum: -maxInt - 1, Maximum: maxInt})
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

func TestClippedSliderUsesTheWholeTrackForValues(t *testing.T) {
	slider := base.NewSlider(base.SliderConfig{Maximum: 100})
	root := newTestRoot(sliderTrack{control: slider, rect: grid.Area(0, 0, 11, 1)})
	root.Draw(grid.NewSurface(6, 1).View())
	if !slider.Handle(input.Mouse{Pos: image.Pt(5, 0), Action: input.MouseDown, Button: input.ButtonLeft}) || slider.Value() != 50 {
		t.Fatalf("value=%d", slider.Value())
	}
	if slider.Handle(input.Mouse{Pos: image.Pt(8, 0), Action: input.MouseDown, Button: input.ButtonLeft}) {
		t.Fatal("clipped part accepted a press")
	}
}
