package kit_test

import (
	"image"
	"testing"

	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/input"
)

func TestSliderComposesLabelTrackThumbAndValue(t *testing.T) {
	slider := kit.NewSlider(kit.Theme{}, kit.Unicode(), "speed", 0, 10)
	slider.Of.Set(5)
	equalRows(t, paintWidget(20, 1, slider), []string{"speed.─────●──────.5"})

	if !slider.Handle(input.Mouse{Pos: image.Pt(17, 0), Action: input.MouseDown, Button: input.ButtonLeft}) {
		t.Fatal("slider declined the end of its visible track")
	}
	if got := slider.Of.Value(); got != 10 {
		t.Fatalf("press on track end chose %d, want 10", got)
	}
	if slider.Handle(input.Mouse{Pos: image.Pt(2, 0), Action: input.MouseDown, Button: input.ButtonLeft}) {
		t.Fatal("slider accepted a press on its label")
	}
}

func TestSliderMayFormatItsValue(t *testing.T) {
	slider := kit.NewSlider(kit.Theme{}, kit.ASCII(), "workers", 1, 8)
	slider.Of.Set(3)
	slider.Format = func(value int) string { return string(rune('0'+value)) + "x" }
	equalRows(t, paintWidget(18, 1, slider), []string{"workers.-O-----.3x"})
}
