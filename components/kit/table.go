package kit

import (
	"image"

	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/layout"
	"github.com/Tangerg/oolong/core/text"
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
	// Fit makes the column ask for the widest title or cell. Width still wins when
	// both are set. Without Width or Fit, the column takes a flexible share.
	Fit bool
	// Max caps a content-fitted column. Zero leaves it uncapped.
	Max int
}

// Cell is one table cell's intrinsic width and drawing behaviour.
//
// Keeping the two together is what makes content-fitted columns trustworthy: the
// value measured is the value later drawn. Preferred is a request rather than a
// reservation; a narrow table may still give the cell less room.
type Cell struct {
	Preferred int
	Paint     func(view grid.View, base grid.Style)
}

// NewCell builds a cell from its preferred width and painter.
func NewCell(preferred int, paint func(view grid.View, base grid.Style)) Cell {
	return Cell{Preferred: max(preferred, 0), Paint: paint}
}

// LabelCell adapts a [Label] into a measured table cell.
//
// The row's base style is merged under the label's style, so a selection or band is
// retained unless the label deliberately replaces it.
func LabelCell(label Label) Cell {
	style := label.Style
	return NewCell(text.Width(label.Text), func(view grid.View, base grid.Style) {
		shown := label
		shown.Style = base.Merge(style)
		shown.Draw(view)
	})
}

// Measure reports the cell's intrinsic width.
func (c Cell) Measure(int) int { return max(c.Preferred, 0) }

// Draw paints the cell over its row style.
func (c Cell) Draw(view grid.View, base grid.Style) {
	if c.Paint != nil {
		c.Paint(view, base)
	}
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
	// Cell returns one cell for a row and column. The value carries both the width a
	// fitted column measures and the painter given its final box.
	//
	// The plain-text case is:
	//
	//	Cell: func(row, col int) kit.Cell {
	//		return kit.LabelCell(kit.Label{Text: data[row][col],
	//			Align: columns[col].Align, Ellipsis: "…"})
	//	}
	//
	// A custom cell uses [NewCell]. Its painter receives the row's base style — a
	// band or selection — because replacing the cells over a filled row without it
	// would erase the band.
	Cell func(row, column int) Cell
	// Gap is the space between columns. Zero uses one column, which is the least
	// that still reads as two columns rather than one.
	Gap int
	// Header draws the column titles in the first row.
	Header bool
	// Sorted says which column the rows are in the order of and which way round, when
	// they are in an order at all, so the header can mark it — the only way a reader
	// can tell an order from a coincidence.
	//
	// It is a function because that is the shape the answer already has: a table with
	// a cursor answers exactly this, so wiring the two together is
	// Sorted: rows.Sorted. And because the zero value of a column number is a column,
	// which would mark the first one on every table nobody sorted.
	Sorted func() (column int, descending, ok bool)
	// Glyphs are the marks beside a sorted column's title. A table given none marks
	// nothing, which is the rule the whole package keeps.
	Glyphs Glyphs
	// RowStyle styles a whole row, for banding or for a selection.
	RowStyle func(row int) grid.Style
}

// TableLayout is one table's column geometry at one width.
//
// Computing it once matters for content-fitted columns: finding the widest cell is
// linear in the rows, and drawing every row must not repeat that scan. A composed
// table also keeps this value as its committed hit-test geometry.
type TableLayout struct {
	table Table
	boxes []image.Rectangle
}

// Layout measures the columns and fixes their boxes at width.
func (t Table) Layout(width int) TableLayout {
	boxes := t.flow().Rects(image.Pt(max(width, 0), 1), t.slots())
	return TableLayout{table: t, boxes: boxes}
}

// Widths returns the column widths. The caller owns the result.
func (l TableLayout) Widths() []int {
	widths := make([]int, len(l.boxes))
	for i, box := range l.boxes {
		widths[i] = box.Dx()
	}
	return widths
}

// Widths works out each column's width for a total. Code drawing more than one row
// should keep [Table.Layout] instead of measuring the content again per row.
func (t Table) Widths(total int) []int { return t.Layout(total).Widths() }

// slots say what each column asks for. A column with neither a width nor a share
// gets one share, because a column nobody sized still has to be visible.
func (t Table) slots() []layout.Slot {
	slots := make([]layout.Slot, len(t.Columns))
	for i, c := range t.Columns {
		if c.Width > 0 {
			slots[i] = layout.Slot{Size: layout.Fixed(c.Width)}
			continue
		}
		if c.Fit {
			column := i
			slots[i] = layout.Slot{
				Size: layout.Measured(c.Min, c.Max),
				Of: layout.MeasureFunc(func(int) int {
					return t.preferred(column)
				}),
			}
			continue
		}
		slots[i] = layout.Slot{Size: layout.Sizing{Flex: max(c.Flex, 1), Min: c.Min}}
	}
	return slots
}

func (t Table) preferred(column int) int {
	c := t.Columns[column]
	widest := text.Width(c.Title)
	if t.Sorted != nil {
		widest += max(text.Width(t.Glyphs.Ascending), text.Width(t.Glyphs.Descending))
	}
	if t.Cell == nil {
		return widest
	}
	for row := range max(t.Rows, 0) {
		widest = max(widest, t.Cell(row, column).Measure(1))
	}
	return widest
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
	y := 0
	columns := t.Layout(width)
	if t.Header {
		columns.Titles(v)
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
		columns.Cells(v.Sub(grid.Rect(0, y, width, 1)), row, band)
	}
}

// Titles draws the column headings into the first row of v, with a mark beside the
// one the rows are sorted by.
//
// It is separate from [Table.Draw] because a table with a cursor draws its own rows:
// the rows are a window onto more of them than fit, and only the thing that owns the
// cursor knows which of them are showing. The header is still this table's, and so
// is where every column starts — which is the whole reason to hand the two out
// separately instead of making a second table that agrees with this one by hand.
func (t Table) Titles(v grid.View) {
	width, _ := v.Size()
	t.Layout(width).Titles(v)
}

// Titles draws the headings using this layout.
func (l TableLayout) Titles(v grid.View) {
	l.drawRow(v, 0, func(col int, cell grid.View) {
		c := l.table.Columns[col]
		Label{Text: c.Title + l.table.mark(col), Style: l.table.Theme.Heading, Align: c.Align, Ellipsis: "…"}.
			Draw(cell)
	})
}

// Cells draws one row's cells into v, which is one row of a table this wide.
//
// base is what the row is drawn on — a band, a selection — and is handed to every
// cell for the reason [Table.Cell] gives: a cell that ignores it loses the row it
// sits in.
func (t Table) Cells(v grid.View, row int, base grid.Style) {
	width, _ := v.Size()
	t.Layout(width).Cells(v, row, base)
}

// Cells draws one row using this layout.
func (l TableLayout) Cells(v grid.View, row int, base grid.Style) {
	if l.table.Cell == nil {
		return
	}
	l.drawRow(v, 0, func(col int, view grid.View) {
		l.table.Cell(row, col).Draw(view, base)
	})
}

// ColumnAt is which column a position in a row of this width falls in, and whether
// it fell in one at all — a press in the gap between two columns is in neither.
//
// It is what turns a click on a heading into a sort. Answering it here is the point
// of the geometry living in one place: a caller working it out from the widths would
// be doing the same arithmetic a second time, against a table that may since have
// been given a different width.
func (t Table) ColumnAt(x, width int) (int, bool) {
	return t.Layout(width).ColumnAt(x)
}

// ColumnAt reports which column contains x in this layout.
func (l TableLayout) ColumnAt(x int) (int, bool) {
	for i, box := range l.boxes {
		if x >= box.Min.X && x < box.Max.X {
			return i, true
		}
	}
	return 0, false
}

// mark is what goes after a column's title to say the rows are in its order.
func (t Table) mark(column int) string {
	if t.Sorted == nil {
		return ""
	}
	sorted, descending, ok := t.Sorted()
	if !ok || sorted != column {
		return ""
	}
	if descending {
		return t.Glyphs.Descending
	}
	return t.Glyphs.Ascending
}

// drawRow hands each column's box to draw, on the row at y.
//
// The boxes are worked out once for the table and moved down a row at a time: where
// a column starts does not depend on which row is being drawn, and a table that
// divided its width again for every row would be doing the same arithmetic once per
// row of the terminal.
func (l TableLayout) drawRow(v grid.View, y int, draw func(col int, cell grid.View)) {
	for col, box := range l.boxes {
		if box.Dx() > 0 {
			draw(col, v.Sub(grid.Rect(box.Min.X, y, box.Dx(), 1)))
		}
	}
}
