package kit_test

import (
	"image"
	"strconv"
	"testing"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/internal/scenetest"
	"github.com/Tangerg/oolong/ui/kit"
)

func TestSliderComposesLabelTrackThumbAndValue(t *testing.T) {
	scenes := scenetest.New()
	t.Cleanup(scenes.Close)

	slider := kit.NewSlider(kit.SliderConfig{Behavior: base.SliderConfig{Maximum: 10, Label: "speed"}, Glyphs: kit.Unicode()})
	slider.Controller().Set(5)
	equalRows(t, paintWidget(scenes, 20, 1, slider), []string{"speed.─────●──────.5"})

	if !scenes.Mount(slider).Handle(input.Mouse{Pos: image.Pt(17, 0), Action: input.MouseDown, Button: input.ButtonLeft}) {
		t.Fatal("slider declined the end of its visible track")
	}
	if got := slider.Controller().Value(); got != 10 {
		t.Fatalf("press on track end chose %d, want 10", got)
	}
	if scenes.Mount(slider).Handle(input.Mouse{Pos: image.Pt(2, 0), Action: input.MouseDown, Button: input.ButtonLeft}) {
		t.Fatal("slider accepted a press on its label")
	}
}

func TestSliderMayFormatItsValue(t *testing.T) {
	scenes := scenetest.New()
	t.Cleanup(scenes.Close)

	slider := kit.NewSlider(kit.SliderConfig{Behavior: base.SliderConfig{Minimum: 1, Maximum: 8, Label: "workers"}, Glyphs: kit.ASCII()})
	slider.Controller().Set(3)
	slider.Format = func(value int) string { return strconv.Itoa(value) + "x" }
	equalRows(t, paintWidget(scenes, 18, 1, slider), []string{"workers.-O-----.3x"})
}

func TestSliderNeverClipsAFormattedValueIntoAnotherNumber(t *testing.T) {
	scenes := scenetest.New()
	t.Cleanup(scenes.Close)

	slider := kit.NewSlider(kit.SliderConfig{Behavior: base.SliderConfig{Maximum: 100}, Glyphs: kit.ASCII()})
	slider.Controller().Set(50)
	slider.Format = func(int) string { return "7000%" }
	equalRows(t, paintWidget(scenes, 3, 1, slider), []string{"-O-"})
	equalRows(t, paintWidget(scenes, 5, 1, slider), []string{"7000%"})
}
