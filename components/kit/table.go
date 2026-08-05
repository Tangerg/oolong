package kit

import (
	"image"

	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/layout"
)

// Column is one column of a [Table].
type Column struct {
	Title string
	Align layout.Align
	// Width is an exact number of columns. Zero lets the column take a share of what
	// is left, weighted by Flex.
	Width int
	// Flex is this column's share of the space the fixed columns did not take. A
	// column with neither a width nor a flex share gets one share, because a column
	// nobody sized still has to be visible.
	Flex int
	// Min is a floor on a flexible column, so it does not collapse to nothing on a
	// narrow terminal.
	Min int
}

// Table lays out rows of cells in columns.
//
// It is layout, not storage: the caller says how many rows there are and what a cell
// contains, and the table works out where each one goes. That keeps the row data
// wherever it already lives instead of copied into a widget, and it means a cell can
// be as elaborate as its own draw function likes.
type Table struct {
	// Theme is the look. A column title is a heading and there is nothing else here
	// with a fixed role: what a row looks like is data, not a look, which is why
	// RowStyle is a function of the row.
	Theme   Theme
	Columns []Column
	// Rows is how many rows there are.
	Rows int
	// Cell draws one cell into a view of exactly its box.
	//
	// There is one way to fill a cell rather than two, and the plain-text case goes
	// through the same door as every other:
	//
	//	Cell: func(v grid.View, row, col int, base grid.Style) {
	//		Label{Text: data[row][col], Align: cols[col].Align,
	//			Style: base, Ellipsis: "…"}.Draw(v)
	//	}
	//
	// base is the row's own style — a band, a selection — already merged with
	// nothing else. A cell drawn over a filled row replaces what was there,
	// background and all, so a cell that ignores base loses the band it sits in.
	Cell func(v grid.View, row, column int, base grid.Style)
	// Gap is the space between columns. Zero uses one column, which is the least
	// that still reads as two columns rather than one.
	Gap int
	// Header draws the column titles in the first row.
	Header bool
	// RowStyle styles a whole row, for banding or for a selection.
	RowStyle func(row int) grid.Style
}

// Widths works out each column's width for a total, which a caller needs when it is
// aligning something else to the same grid.
func (t Table) Widths(total int) []int {
	return t.flow().Divide(total, 1, t.slots())
}

// slots say what each column asks for. A column with neither a width nor a share
// gets one share, because a column nobody sized still has to be visible.
func (t Table) slots() []layout.Slot {
	slots := make([]layout.Slot, len(t.Columns))
	for i, c := range t.Columns {
		if c.Width > 0 {
			slots[i] = layout.Slot{Size: layout.Fixed(c.Width)}
			continue
		}
		slots[i] = layout.Slot{Size: layout.Sizing{Flex: max(c.Flex, 1), Min: c.Min}}
	}
	return slots
}

// flow is how the columns divide the width: across, with the gap between them.
//
// The arithmetic used to be written out here — the fixed columns, then the shares of
// what was left, then the rounding remainder to the last one. It was the same
// arithmetic as [layout.Divide] with the gaps taken off the front, which is the
// signal that a gap belongs one layer down: two copies of a sizing rule are two
// chances to round it differently.
func (t Table) flow() layout.Flow {
	return layout.Flow{Axis: layout.Across, Gap: max(t.Gap, 1)}
}

// Measure is the rows plus the header, which is what a container measures against.
func (t Table) Measure(int) int {
	if t.Header {
		return t.Rows + 1
	}
	return t.Rows
}

// Draw paints the header and as many rows as fit.
func (t Table) Draw(v grid.View) {
	width, height := v.Size()
	if width <= 0 || height <= 0 || len(t.Columns) == 0 {
		return
	}
	boxes := t.flow().Rects(layout.Size{W: width, H: 1}, t.slots())

	y := 0
	if t.Header {
		t.drawRow(v, y, boxes, func(col int, cell grid.View) {
			c := t.Columns[col]
			Label{Text: c.Title, Style: t.Theme.Heading, Align: c.Align, Ellipsis: "…"}.Draw(cell)
		})
		y++
	}
	if t.Cell == nil {
		return
	}
	for row := 0; row < t.Rows && y < height; row, y = row+1, y+1 {
		var band grid.Style
		if t.RowStyle != nil {
			band = t.RowStyle(row)
		}
		if band != (grid.Style{}) {
			v.Fill(grid.Rect(0, y, width, 1), band)
		}
		t.drawRow(v, y, boxes, func(col int, cell grid.View) {
			t.Cell(cell, row, col, band)
		})
	}
}

// drawRow hands each column's box to draw, on the row at y.
//
// The boxes are worked out once for the table and moved down a row at a time: where
// a column starts does not depend on which row is being drawn, and a table that
// divided its width again for every row would be doing the same arithmetic once per
// row of the terminal.
func (t Table) drawRow(v grid.View, y int, boxes []image.Rectangle, draw func(col int, cell grid.View)) {
	for col, box := range boxes {
		if box.Dx() > 0 {
			draw(col, v.Sub(grid.Rect(box.Min.X, y, box.Dx(), 1)))
		}
	}
}
