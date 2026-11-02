package kit_test

import (
	"image"
	"testing"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/kit"
)

func TestPanelMeasuresWidthAtItsContentHeight(t *testing.T) {
	for _, height := range []int{10, 6, 2, 0} {
		var measuredHeight int
		var painted image.Point
		content := scene.NewElement(scene.ElementConfig{
			Measure: func(axis layout.Axis, across int) int {
				if axis == layout.Across {
					measuredHeight = across
					return 3 * across
				}
				return 1
			},
			Paint: func(frame scene.Frame) { painted = frame.Bounds().Size() },
		})
		panel := kit.NewPanel(kit.PanelConfig{
			Box: kit.Box{Glyphs: kit.ASCII(), Padding: layout.Symmetric(2, 3)}, Content: content,
		})
		width := panel.Element().Measure(layout.Across, height)
		contentHeight := max(height-6, 0)
		if measuredHeight != contentHeight || width != 8+3*contentHeight {
			t.Fatalf("height %d: child measured at %d, width %d; want %d and %d", height, measuredHeight, width, contentHeight, 8+3*contentHeight)
		}
		root, err := scene.NewRoot(scene.RootConfig{Element: panel.Element()})
		if err != nil {
			t.Fatal(err)
		}
		frame, err := root.Prepare(grid.NewSurface(width, height).View())
		if err != nil {
			t.Fatal(err)
		}
		frame.Abort()
		root.Close()
		if contentHeight > 0 && painted != image.Pt(3*contentHeight, contentHeight) {
			t.Fatalf("painted content %v differs from measured interior", painted)
		}
	}
}
