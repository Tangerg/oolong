package markdown

import (
	"strings"

	east "github.com/yuin/goldmark/extension/ast"

	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/layout"
	"github.com/Tangerg/oolong/core/text"
)

// tableColumnFloor is the smallest column that still reads as a column. A table
// that cannot afford this much room per non-empty column becomes records instead
// of preserving a grid whose cells are mostly vertical fragments.
const tableColumnFloor = 4

type table struct {
	rows   [][]text.Line
	aligns []columnAlignment
	header bool

	separator string
	divider   string
	rail      grid.Style
	rule      grid.Style
}

type columnAlignment uint8

const (
	alignLeft columnAlignment = iota
	alignCenter
	alignRight
)

// table retains cells until layout. A table is the one markdown block whose shape
// depends on its contents and on the final region at the same time; flattening it
// during parsing loses the information needed to make a narrow table readable.
func (r *renderer) table(n *east.Table, in frame) {
	rows, header := r.cells(n)
	if len(rows) == 0 {
		return
	}
	r.push(Block{
		indent: in.indent, rail: in.rail.line(), blankBefore: !in.tight,
		table: &table{
			rows: rows, aligns: columnAlignments(n.Alignments), header: header,
			separator: r.column(), divider: r.look.Glyphs.Divider,
			rail: r.look.Rail, rule: r.look.Rule,
		},
	})
}

// cells reads the table into styled cells and says whether the first row is its
// heading. Cells remain logical lines; wrapping belongs to table layout.
func (r *renderer) cells(n *east.Table) (rows [][]text.Line, header bool) {
	for child := n.FirstChild(); child != nil; child = child.NextSibling() {
		style := r.look.Text
		if _, is := child.(*east.TableHeader); is {
			style = r.look.Strong
			header = header || len(rows) == 0
		}
		row := make([]text.Line, 0, 8)
		for cell := child.FirstChild(); cell != nil; cell = cell.NextSibling() {
			// Markdown table syntax cannot express a line break inside a cell. An
			// empty cell can still produce no line, so make that case an empty one.
			lines := r.inline(cell, style)
			if len(lines) == 0 {
				row = append(row, nil)
				continue
			}
			row = append(row, lines[0])
		}
		rows = append(rows, row)
	}
	return rows, header
}

func columnAlignments(in []east.Alignment) []columnAlignment {
	out := make([]columnAlignment, len(in))
	for i, alignment := range in {
		switch alignment {
		case east.AlignCenter:
			out[i] = alignCenter
		case east.AlignRight:
			out[i] = alignRight
		case east.AlignLeft, east.AlignNone:
			out[i] = alignLeft
		}
	}
	return out
}

func (t *table) appendRows(dst []row, room int) []row {
	widths, grid := t.columnWidths(room)
	if grid {
		return t.appendGrid(dst, widths)
	}
	return t.appendRecords(dst, room)
}

// columnWidths allocates the room left after separators. Compact columns reach
// their natural width first; columns still growing share the remainder. It reports
// false when even the readable floors do not fit, which is the point at which a
// record layout communicates the table better than a grid.
func (t *table) columnWidths(room int) ([]int, bool) {
	natural := t.naturalWidths()
	if len(natural) == 0 {
		return nil, true
	}
	budget := layout.Remaining(room, t.separatorWidth(len(natural)))
	widths := make([]int, len(natural))
	used := 0
	for i, width := range natural {
		widths[i] = min(width, tableColumnFloor)
		used = layout.Sum(used, widths[i])
	}
	if used > budget {
		return nil, false
	}
	t.growColumns(widths, natural, budget-used)
	return widths, true
}

func (t *table) naturalWidths() []int {
	widths := make([]int, t.columns())
	for _, row := range t.rows {
		for column, cell := range row {
			widths[column] = max(widths[column], cell.Width())
		}
	}
	return widths
}

func (t *table) separatorWidth(columns int) int {
	width := 0
	for range max(columns-1, 0) {
		width = layout.Sum(width, text.Width(t.separator))
	}
	return width
}

// growColumns water-fills every column still below its natural width. A column that
// reaches its target leaves the next round, so compact columns do not consume the
// same share as content that can still use it.
func (t *table) growColumns(widths, natural []int, remaining int) {
	pending := make([]int, 0, len(widths))
	for i := range widths {
		if widths[i] < natural[i] {
			pending = append(pending, i)
		}
	}
	for remaining > 0 && len(pending) > 0 {
		share := remaining / len(pending)
		if share == 0 {
			for _, column := range pending[:min(remaining, len(pending))] {
				widths[column]++
			}
			break
		}

		next := pending[:0]
		settled := false
		for _, column := range pending {
			need := natural[column] - widths[column]
			if need <= share {
				widths[column] += need
				remaining -= need
				settled = true
				continue
			}
			next = append(next, column)
		}
		if settled {
			pending = next
			continue
		}

		for _, column := range pending {
			widths[column] += share
			remaining -= share
		}
		for _, column := range pending[:min(remaining, len(pending))] {
			widths[column]++
		}
		break
	}
}

func (t *table) columns() int {
	columns := 0
	for _, row := range t.rows {
		columns = max(columns, len(row))
	}
	return columns
}

func (t *table) appendGrid(dst []row, widths []int) []row {
	for rowIndex, cells := range t.rows {
		wrapped := make([][]text.Wrapped, len(widths))
		height := 1
		for column, width := range widths {
			var cell text.Line
			if column < len(cells) {
				cell = cells[column]
			}
			wrapped[column] = cell.Wrap(width)
			height = max(height, len(wrapped[column]))
		}
		for physical := range height {
			line := make(text.Line, 0, len(widths)*3)
			for column, width := range widths {
				if column > 0 {
					line = append(line, text.Span{Text: t.separator, Style: t.rail})
				}
				var cell text.Line
				if physical < len(wrapped[column]) {
					cell = wrapped[column][physical].Line
				}
				line = appendAligned(line, cell, width, t.alignment(column))
			}
			dst = append(dst, row{Line: line})
		}
		if rowIndex == 0 && t.header {
			dst = append(dst, row{Line: t.ruleLine(widths)})
		}
	}
	return dst
}

func appendAligned(dst text.Line, line text.Line, width int, alignment columnAlignment) text.Line {
	padding := max(width-line.Width(), 0)
	left, right := 0, padding
	switch alignment {
	case alignRight:
		left, right = padding, 0
	case alignCenter:
		left, right = padding/2, padding-padding/2
	case alignLeft:
	}
	if left > 0 {
		dst = append(dst, blank(left))
	}
	dst = append(dst, line...)
	if right > 0 {
		dst = append(dst, blank(right))
	}
	return dst
}

func (t *table) ruleLine(widths []int) text.Line {
	divider := t.divider
	if divider == "" {
		divider = " "
	}
	out := make(text.Line, 0, len(widths)*2)
	for i, width := range widths {
		if i > 0 {
			out = append(out, text.Span{Text: t.separator, Style: t.rail})
		}
		out = append(out, text.Span{Text: strings.Repeat(divider, width), Style: t.rule})
	}
	return out
}

// appendRecords uses the heading cells as field names. No labels are invented for
// a table without a heading; each cell simply gets a line of its own. A blank row
// keeps adjacent records distinct without introducing product-specific furniture.
func (t *table) appendRecords(dst []row, room int) []row {
	var keys []text.Line
	records := t.rows
	if t.header && len(t.rows) > 1 {
		keys, records = t.rows[0], t.rows[1:]
	}
	for recordIndex, record := range records {
		if recordIndex > 0 {
			dst = append(dst, row{})
		}
		for column := range max(len(keys), len(record)) {
			var key, value text.Line
			if column < len(keys) {
				key = keys[column]
			}
			if column < len(record) {
				value = record[column]
			}
			line := make(text.Line, 0, len(key)+len(value)+1)
			line = append(line, key...)
			if len(key) > 0 {
				line = append(line, text.Span{Text: ": ", Style: t.rail})
			}
			line = append(line, value...)
			dst = appendWrapped(dst, line, room)
		}
	}
	return dst
}

func (t *table) alignment(column int) columnAlignment {
	if column < 0 || column >= len(t.aligns) {
		return alignLeft
	}
	return t.aligns[column]
}

// column is what goes between two cells: the look's bar where it has one, and room
// where it does not — because two columns run together is worse than a table with no
// lines in it.
func (r *renderer) column() string {
	if r.look.Glyphs.Bar == "" {
		return "  "
	}
	return " " + r.look.Glyphs.Bar + " "
}

// blank is n columns of nothing.
func blank(n int) text.Span {
	if n <= 0 {
		return text.Span{}
	}
	return text.Span{Text: strings.Repeat(" ", n)}
}
