package kit

import (
	"image"
	"slices"
	"strings"

	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/layout"
	"github.com/Tangerg/oolong/core/text"
)

// Column is one column of a [Table].
type Column struct {
	Title string
	Align layout.Align
	// Size is how the shared layout allocator sizes this column. Use layout.Fixed
	// for an exact width, layout.Flex for a share of what remains, and
	// layout.Measured to fit the widest title or cell. The zero value defaults to
	// layout.Flex(1), so an unsized column participates in the remaining space.
	Size layout.Sizing
}

// Cell is one table cell's intrinsic width and drawing behaviour.
//
// Keeping the two together is what makes content-fitted columns trustworthy: the
// value measured is the value later drawn. Preferred is a request rather than a
// reservation; a narrow table may still give the cell less room.
type Cell struct {
	Preferred int
	// Paint runs during drawing and must only paint its view; it must not mutate
	// application state, publish output, or start work.
	Paint func(view grid.View, base grid.Style)
}

// LabelCell adapts a [Label] into a measured table cell.
//
// The row's base style is merged under the label's style, so a selection or band is
// retained unless the label deliberately replaces it.
func LabelCell(label Label) Cell {
	style := label.Style
	return Cell{
		Preferred: text.Width(label.Text),
		Paint: func(view grid.View, base grid.Style) {
			shown := label
			shown.Style = base.Merge(style)
			shown.Draw(view)
		},
	}
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
	// A custom cell uses a Cell literal. Its painter receives the row's base style —
	// a band or selection — because replacing the cells over a filled row without it
	// would erase the band. Cell and Paint are projection callbacks: they may run
	// repeatedly during measurement and drawing and must be observationally pure.
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
	// Sorted may run during drawing and must only observe ordering state.
	Sorted func() (column int, descending, ok bool)
	// Glyphs are the marks beside a sorted column's title. A table given none marks
	// nothing, which is the rule the whole package keeps.
	Glyphs Glyphs
	// RowStyle styles a whole row, for banding or for a selection. It is a pure
	// projection callback and may run during drawing.
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
	t.Columns = slices.Clone(t.Columns)
	for i := range t.Columns {
		t.Columns[i].Title = strings.Clone(t.Columns[i].Title)
	}
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

// slots say what each column asks for. An omitted policy gets one flexible share,
// which makes the zero Column useful without maintaining another sizing scheme here.
func (t Table) slots() []layout.Slot {
	slots := make([]layout.Slot, len(t.Columns))
	for i, c := range t.Columns {
		size := c.Size
		if size.IsZero() {
			size = layout.Flex(1)
		}
		column := i
		slots[i] = layout.Slot{
			Size: size,
			Of: layout.MeasureFunc(func(int) int {
				return t.preferred(column)
			}),
		}
	}
	return slots
}

func (t Table) preferred(column int) int {
	c := t.Columns[column]
	widest := text.Width(c.Title + t.mark(column))
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
// arithmetic as [layout.Flow.Divide] with the gaps taken off the front, which is the
// signal that a gap belongs one layer down: two copies of a sizing rule are two
// chances to round it differently.
func (t Table) flow() layout.Flow {
	return layout.Flow{Axis: layout.Across, Gap: max(t.Gap, 1)}
}

// Measure is the rows plus the header, which is what a container measures against.
func (t Table) Measure(int) int {
	rows := max(t.Rows, 0)
	if t.Header {
		rows = layout.Sum(rows, 1)
	}
	return rows
}

// Draw paints the header and as many rows as fit.
func (t Table) Draw(v grid.View) {
	if v.Empty() {
		return
	}
	width, height := v.Size()
	if width <= 0 || height <= 0 || len(t.Columns) == 0 {
		return
	}
	columns := t.Layout(width)
	visible := v.Visible()
	first := min(max(visible.Min.Y, 0), height)
	last := min(max(visible.Max.Y, first), height)
	header := 0
	if t.Header {
		header = 1
	}
	if t.Header && first == 0 && last > 0 {
		columns.Titles(v)
	}
	if t.Cell == nil {
		return
	}
	firstRow := min(max(first-header, 0), max(t.Rows, 0))
	lastRow := min(max(last-header, firstRow), max(t.Rows, 0))
	for row := firstRow; row < lastRow; row++ {
		y := row + header
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

// Titles draws the headings using this layout. A table with a cursor keeps the
// layout for its visible rows and for committed heading hit tests, so all three use
// the same boxes.
func (l TableLayout) Titles(v grid.View) {
	l.drawRow(v, 0, func(col int, cell grid.View) {
		c := l.table.Columns[col]
		Label{Text: c.Title + l.table.mark(col), Style: l.table.Theme.Heading, Align: c.Align, Ellipsis: "…"}.
			Draw(cell)
	})
}

// Cells draws one row using this layout. Base is the row's band or selection and is
// handed to every cell for the reason [Table.Cell] gives.
func (l TableLayout) Cells(v grid.View, row int, base grid.Style) {
	if l.table.Cell == nil {
		return
	}
	l.drawRow(v, 0, func(col int, view grid.View) {
		l.table.Cell(row, col).Draw(view, base)
	})
}

// ColumnAt reports which column contains x in this layout. A press in a gap is in
// neither. Keeping the accepted frame's layout makes a heading click answer about
// the same boxes the reader saw.
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
