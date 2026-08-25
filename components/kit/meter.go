package kit

import (
	"image"
	"math"

	"github.com/Tangerg/oolong/core/grid"
)

// meterLayout is the shared one-row geometry of a label, a changing visual extent,
// and a right-aligned value. Progress, Slider, and BarChart differ in behavior and
// appearance; they do not differ in how those three parts make room for one another.
// A value is atomic: when it cannot fit in full, its column disappears and gives the
// room back to the track. A clipped number can look like a different, valid number.
type meterLayout struct {
	label image.Rectangle
	track image.Rectangle
	value image.Rectangle
}

func layoutMeter(width, labelWidth, valueWidth int) meterLayout {
	width = max(width, 0)
	valueWidth = max(valueWidth, 0)
	if valueWidth > width {
		valueWidth = 0
	}
	value := grid.Rect(width-valueWidth, 0, valueWidth, 1)

	trackEnd := value.Min.X
	if valueWidth > 0 && trackEnd > 0 {
		trackEnd-- // air between the changing extent and its value
	}
	labelWidth = min(max(labelWidth, 0), min(width/2, trackEnd))
	label := grid.Rect(0, 0, labelWidth, 1)
	trackStart := label.Max.X
	if labelWidth > 0 && trackStart < trackEnd {
		trackStart++ // air between a name and what it names
	}
	return meterLayout{
		label: label,
		track: image.Rect(trackStart, 0, max(trackStart, trackEnd), 1),
		value: value,
	}
}

// bar is the one cell-precise horizontal extent shared by every kit meter. Progress
// and BarChart supply different data and surrounding labels; the terminal geometry
// of a filled fraction has one implementation. Partial-cell steps keep a narrow bar
// from moving in whole-column jumps; a glyph set without them deliberately degrades
// to whole cells.
type bar struct {
	fraction float64
	glyphs   Glyphs
	full     grid.Style
	empty    grid.Style
}

func (b bar) Draw(v grid.View) {
	w, _ := v.Size()
	if w <= 0 || b.glyphs.BarFull == "" || b.glyphs.BarEmpty == "" {
		return
	}
	fraction := b.fraction
	switch {
	case math.IsNaN(fraction), fraction <= 0:
		fraction = 0
	case fraction >= 1:
		fraction = 1
	}
	filled := fraction * float64(w)
	whole := int(filled)
	steps := b.glyphs.BarSteps
	step := 0
	if len(steps) > 0 {
		step = min(int((filled-float64(whole))*float64(len(steps)+1)), len(steps))
	}
	for x := range w {
		switch {
		case x < whole:
			v.Text(x, 0, b.glyphs.BarFull, b.full)
		case x == whole && step > 0:
			v.Text(x, 0, steps[step-1], b.full)
		default:
			v.Text(x, 0, b.glyphs.BarEmpty, b.empty)
		}
	}
}
