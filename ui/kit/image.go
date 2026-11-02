package kit

import (
	"image"

	"github.com/Tangerg/oolong/ui/core/graphics"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/text"
)

// Image shows a picture the terminal has already been given.
//
// The sending happens once in the host adapter, which hands back the handle that
// goes in [Image.Of]. This value places it: it works out
// how many cells the picture should have at the width it is drawn in, keeps the room
// for it, and lets the frame do the rest.
//
// # When there is no picture to show
//
// Which is often. A terminal may not show pictures at all, may not say how big a
// cell is, or may be an interface being written to a file. All three end in the same
// place, and [Image.Alt] is what is drawn there — the alternative text a document
// gave, which is exactly what it was written for.
//
// Whether the terminal can is not this widget's to ask: it is one question at
// startup and the answer is a program's, so a caller that passes a handle it never
// obtained gets the alternative and no harm.
type Image struct {
	// Of is the picture, as the terminal knows it. The zero value is no picture,
	// which draws the alternative text.
	Of graphics.Resource
	// Placement is the placement identity within Of. Zero uses 1; simultaneous
	// views of one image must choose distinct nonzero identities.
	Placement uint32
	// Source selects a rectangle in the original PNG's pixels. Zero displays the whole
	// image. A selection outside the image draws Alt.
	Source image.Rectangle
	// Cell is how many pixels one terminal cell is, as reported by the host.
	// The zero value is a terminal that never said, which draws the alternative text:
	// a picture scaled by an invented cell size is a picture the wrong shape.
	Cell image.Point
	// MaxRows caps how tall the picture may be, so one from somewhere else cannot
	// take the whole screen. Zero is a cap of eight rows, which is about a third of a
	// terminal and small enough to read around.
	MaxRows int

	// Alt is what to say where the picture cannot be shown.
	Alt string
	// Align is where a picture narrower than its space sits.
	Align  layout.Align
	Theme  Theme
	Glyphs Glyphs
}

// defaultMaxRows is how tall a picture is allowed to be when nothing said: about a
// third of a terminal, which leaves room to read around it.
const defaultMaxRows = 8

// HeightForWidth is how many rows the picture takes at this width, or one row for the
// alternative text where it cannot be shown.
func (i Image) HeightForWidth(width int) int {
	_, cells := i.fit(width, i.rowLimit())
	return max(1, cells.Y)
}

// Draw keeps the room the picture needs, or writes the alternative text.
func (i Image) Draw(v grid.View) {
	width, height := v.Size()
	if width <= 0 || height <= 0 {
		return
	}
	placement, cells := i.fit(width, min(height, i.rowLimit()))
	if cells.Y == 0 {
		Label{Text: i.Alt, Style: i.Theme.Muted, Align: i.Align, Ellipsis: i.Glyphs.Ellipsis}.Draw(v)
		return
	}
	at := i.Align.Offset(width, cells.X)
	placement.Draw(v, grid.Area(at, 0, cells.X, cells.Y))
}

func (i Image) fit(width, height int) (graphics.ResourcePlacement, image.Point) {
	if !i.Of.Live() || i.Cell.X <= 0 || i.Cell.Y <= 0 || width <= 0 {
		return graphics.ResourcePlacement{}, image.Point{}
	}
	placement := i.Of.Placement(max(i.Placement, 1))
	placement.Source = i.Source
	bounds := placement.SourceBounds()
	if bounds.Empty() {
		return graphics.ResourcePlacement{}, image.Point{}
	}
	return placement, graphics.Fit(bounds.Size(), i.Cell, image.Pt(width, height))
}

func (i Image) rowLimit() int {
	if i.MaxRows > 0 {
		return i.MaxRows
	}
	return defaultMaxRows
}

// Width is the cell width of the alternative text. It lets Image satisfy a
// consumer-owned width interface without making the consumer repeat how terminal
// text is measured; the displayed picture itself still fits the width given to Draw.
func (i Image) Width() int { return text.Width(i.Alt) }
