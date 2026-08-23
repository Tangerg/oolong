package kit

import (
	"image"

	"github.com/Tangerg/oolong/core/graphics"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/layout"
	"github.com/Tangerg/oolong/core/text"
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
	Of graphics.Image
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
	Align layout.Align
	// Theme is the look of the alternative text, which is text present for reference
	// rather than for reading.
	Theme Theme
}

// defaultMaxRows is how tall a picture is allowed to be when nothing said: about a
// third of a terminal, which leaves room to read around it.
const defaultMaxRows = 8

// Measure is how many rows the picture takes at this width, or one row for the
// alternative text where it cannot be shown.
func (i Image) Measure(width int) int {
	if _, rows, ok := i.fit(width); ok {
		return rows
	}
	return 1
}

// Draw keeps the room the picture needs, or writes the alternative text.
func (i Image) Draw(v grid.View) {
	width, height := v.Size()
	if width <= 0 || height <= 0 {
		return
	}
	cols, rows, ok := i.fit(width)
	if !ok {
		Label{Text: i.Alt, Style: i.Theme.Muted, Align: i.Align, Ellipsis: "…"}.Draw(v)
		return
	}
	at := i.Align.Offset(width, cols)
	// The identity is the picture's own: two frames that show the same picture in the
	// same place say nothing between them, which is what a terminal wants to hear
	// about something it is already showing.
	v.Paint(grid.Rect(at, 0, cols, min(rows, height)), uint64(i.Of.ID), i.Of)
}

// fit is the box the picture should occupy at a width, and whether there is a
// picture to fit at all.
func (i Image) fit(width int) (cols, rows int, ok bool) {
	if i.Of.ID == 0 || i.Of.Size.X <= 0 || i.Of.Size.Y <= 0 || i.Cell.X <= 0 || i.Cell.Y <= 0 {
		return 0, 0, false
	}
	if width <= 0 {
		return 0, 0, false
	}
	limit := i.MaxRows
	if limit <= 0 {
		limit = defaultMaxRows
	}
	box := graphics.Fit(i.Of.Size, i.Cell, image.Pt(width, limit))
	return box.X, box.Y, true
}

// Width is the cell width of the alternative text. It lets Image satisfy a
// consumer-owned width interface without making the consumer repeat how terminal
// text is measured; the displayed picture itself still fits the width given to Draw.
func (i Image) Width() int { return text.Width(i.Alt) }
