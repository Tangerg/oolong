package kit

import (
	"image"

	"github.com/Tangerg/oolong/core/grid"
)

// meterLayout is the shared one-row geometry of a label, a changing visual extent,
// and a right-aligned value. Progress and Slider differ in behavior and appearance;
// they do not differ in how those three parts make room for one another.
type meterLayout struct {
	label image.Rectangle
	track image.Rectangle
	value image.Rectangle
}

func layoutMeter(width, labelWidth, valueWidth int) meterLayout {
	width = max(width, 0)
	valueWidth = min(max(valueWidth, 0), width)
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
