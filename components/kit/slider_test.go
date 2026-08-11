package kit_test

import (
	"image"
	"strconv"
	"testing"

	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/input"
)

func TestSliderComposesLabelTrackThumbAndValue(t *testing.T) {
	slider := kit.NewSlider(kit.SliderConfig{Glyphs: kit.Unicode(), Maximum: 10, Label: "speed"})
	slider.Controller().Set(5)
	equalRows(t, paintWidget(20, 1, slider), []string{"speed.─────●──────.5"})

	if !slider.Handle(input.Mouse{Pos: image.Pt(17, 0), Action: input.MouseDown, Button: input.ButtonLeft}) {
		t.Fatal("slider declined the end of its visible track")
	}
	if got := slider.Controller().Value(); got != 10 {
		t.Fatalf("press on track end chose %d, want 10", got)
	}
	if slider.Handle(input.Mouse{Pos: image.Pt(2, 0), Action: input.MouseDown, Button: input.ButtonLeft}) {
		t.Fatal("slider accepted a press on its label")
	}
}

func TestSliderMayFormatItsValue(t *testing.T) {
	slider := kit.NewSlider(kit.SliderConfig{Glyphs: kit.ASCII(), Minimum: 1, Maximum: 8, Label: "workers"})
	slider.Controller().Set(3)
	slider.Format = func(value int) string { return strconv.Itoa(value) + "x" }
	equalRows(t, paintWidget(18, 1, slider), []string{"workers.-O-----.3x"})
}
