package grid_test

import (
	"testing"

	"github.com/Tangerg/oolong/ui/core/grid"
)

// The advance a caller chains the next draw from used to depend on whether the
// atom a zero-width cluster joins was inside the clip.
func TestTextAdvanceDoesNotDependOnClipping(t *testing.T) {
	measure := func(clip bool) int {
		surface := grid.NewSurface(4, 2)
		view := surface.View()
		view.Text(0, 0, "☺", grid.Style{})
		if clip {
			return view.Sub(grid.Area(1, 0, 3, 1)).Text(0, 0, "️", grid.Style{})
		}
		return view.Text(1, 0, "️", grid.Style{})
	}
	if visible, clipped := measure(false), measure(true); visible != clipped || visible != 1 {
		t.Fatalf("advance visible=%d clipped=%d, want the join's width change either way", visible, clipped)
	}
	surface := grid.NewSurface(4, 2)
	view := surface.View()
	view.Text(0, 1, "☺", grid.Style{})
	offRow := view.Sub(grid.Area(0, 0, 4, 1)).Text(1, 1, "️", grid.Style{})
	if offRow != 1 {
		t.Fatalf("advance on a clipped row = %d, want 1", offRow)
	}
}
