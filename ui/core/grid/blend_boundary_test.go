package grid_test

import (
	"math"
	"slices"
	"testing"

	"github.com/Tangerg/oolong/ui/core/grid"
)

func TestBlendingRejectsNaNBeforeChangingAppearance(t *testing.T) {
	for _, operation := range []struct {
		name  string
		apply func(grid.View)
	}{
		{"RGB", func(grid.View) { _ = (grid.RGB{R: 100}).Blend(grid.RGB{B: 200}, math.NaN()) }},
		{"Color", func(grid.View) { _ = grid.RGBColor(100, 0, 0).Blend(grid.RGBColor(0, 0, 200), math.NaN()) }},
		{"default color", func(grid.View) { _ = (grid.Color{}).Blend(grid.RGBColor(0, 0, 200), math.NaN()) }},
		{"Blend", func(view grid.View) { view.Blend(view.Bounds(), grid.RGBColor(0, 0, 200), math.NaN()) }},
		{"Fade", func(view grid.View) { view.Fade(view.Bounds(), math.NaN()) }},
	} {
		t.Run(operation.name, func(t *testing.T) {
			surface := grid.NewSurface(4, 1)
			view := surface.View()
			view.Text(0, 0, "text", grid.Style{FG: grid.RGBColor(100, 50, 20), BG: grid.RGBColor(20, 30, 40)})
			before := surface.Row(0)
			defer func() {
				if recover() == nil {
					t.Error("NaN opacity was accepted")
				}
				if !slices.Equal(before, surface.Row(0)) {
					t.Error("invalid opacity changed the surface")
				}
			}()
			operation.apply(view)
		})
	}
}

func TestBlendingClampsOrderedOpacity(t *testing.T) {
	base, over := grid.RGB{R: 100}, grid.RGB{B: 200}
	for _, test := range []struct {
		opacity float64
		want    grid.RGB
	}{
		{math.Inf(-1), base},
		{-1, base},
		{0, base},
		{1, over},
		{2, over},
		{math.Inf(1), over},
	} {
		if got := base.Blend(over, test.opacity); got != test.want {
			t.Errorf("opacity %g: got %v, want %v", test.opacity, got, test.want)
		}
	}
}
