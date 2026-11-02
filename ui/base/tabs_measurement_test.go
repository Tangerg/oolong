package base_test

import (
	"image"
	"testing"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/scene"
)

func TestTabsMeasureWidthAtTheAllocatedPaneHeight(t *testing.T) {
	for _, height := range []int{7, 2, 1, 0} {
		var measuredHeight int
		var painted image.Point
		pane := scene.NewElement(scene.ElementConfig{
			Measure: func(axis layout.Axis, across int) int {
				if axis == layout.Across {
					measuredHeight = across
					return 3 * across
				}
				return 1
			},
			Paint: func(frame scene.Frame) { painted = frame.Bounds().Size() },
		})
		tabs := base.NewTabs(base.TabsConfig{StripRows: func() int { return 2 }, Items: []base.Tab{{Title: "tab", Content: pane}}})
		width := tabs.Element().Measure(layout.Across, height)
		paneHeight := max(height-2, 0)
		wantWidth := max(3, 3*paneHeight)
		if height == 0 {
			wantWidth = 0
		}
		if measuredHeight != paneHeight || width != wantWidth {
			t.Fatalf("height %d: pane measured at %d, width %d; want %d and %d", height, measuredHeight, width, paneHeight, wantWidth)
		}
		root, err := scene.NewRoot(scene.RootConfig{Element: tabs.Element()})
		if err != nil {
			t.Fatal(err)
		}
		frame, err := root.Prepare(grid.NewSurface(width, height).View())
		if err != nil {
			t.Fatal(err)
		}
		frame.Abort()
		root.Close()
		if paneHeight > 0 && painted != image.Pt(width, paneHeight) {
			t.Fatalf("painted pane %v differs from measured allocation", painted)
		}
	}
}
