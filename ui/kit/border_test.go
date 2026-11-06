package kit_test

import (
	"image"
	"testing"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/kit"
)

func TestBoxBorderStaysOutsideItsContent(t *testing.T) {
	accent := newCellGlyph(t, "e\u0301")
	custom := kit.Border{
		Top: accent, Bottom: accent, Left: accent, Right: accent,
		TopLeft: accent, TopRight: accent, BottomLeft: accent, BottomRight: accent,
	}
	for _, tc := range []struct {
		name   string
		border kit.Border
	}{
		{name: "custom grapheme", border: custom},
		{name: "rounded unicode", border: kit.Unicode().Rounded()},
		{name: "square unicode", border: kit.Unicode().Square()},
		{name: "ASCII", border: kit.ASCII().Square()},
		{name: "omitted edges", border: kit.Border{Left: accent, Right: accent}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			surface := grid.NewSurface(5, 3)
			box := kit.Box{Border: tc.border}
			inner := box.Draw(surface.View())
			if width, height := inner.Size(); width != 3 || height != 1 {
				t.Fatalf("content = %dx%d, want 3x1", width, height)
			}
			points := []image.Point{{0, 0}, {2, 0}, {4, 0}, {0, 1}, {4, 1}, {0, 2}, {2, 2}, {4, 2}}
			before := make([]grid.Cell, len(points))
			for i, point := range points {
				before[i] = cellAt(surface, point.X, point.Y)
			}
			inner.Text(0, 0, "界X", grid.Style{})
			for i, point := range points {
				if got := cellAt(surface, point.X, point.Y); got != before[i] {
					t.Fatalf("content changed border at %v: got=%+v want=%+v", point, got, before[i])
				}
			}
		})
	}
}

func TestRulePatternDoesNotChangeBorderGeometry(t *testing.T) {
	glyphs := kit.ASCII()
	glyphs.Rule = "界-"
	box := kit.Box{Glyphs: glyphs}
	equalRows(t, paint(5, 3, func(view grid.View) {
		inner := box.Draw(view)
		inner.Text(0, 0, "abc", grid.Style{})
	}), []string{"+---+", "|abc|", "+---+"})
}
