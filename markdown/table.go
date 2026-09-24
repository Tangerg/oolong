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
			break
		}
		rest, left := settleColumns(widths, natural, pending, share, remaining)
		if len(rest) < len(pending) {
			// Finishing a column first is what lets the width it did not need reach
			// the ones that are still short.
			pending, remaining = rest, left
			continue
		}
		for _, column := range pending {
			widths[column] += share
		}
		remaining -= share * len(pending)
		break
	}
	scatter(widths, pending, remaining)
}

// settleColumns gives every column a share is enough for all it still wants, and
// reports the ones it is not enough for.
func settleColumns(widths, natural, pending []int, share, remaining int) ([]int, int) {
	still := pending[:0]
	for _, column := range pending {
		need := natural[column] - widths[column]
		if need > share {
			still = append(still, column)
			continue
		}
		widths[column] += need
		remaining -= need
	}
	return still, remaining
}

// scatter hands out the columns too few to divide, one each.
func scatter(widths, pending []int, n int) {
	for _, column := range pending[:min(max(n, 0), len(pending))] {
		widths[column]++
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
		dst = t.appendRow(dst, cells, widths)
		if rowIndex == 0 && t.header {
			dst = append(dst, row{Line: t.ruleLine(widths)})
		}
	}
	return dst
}

// appendRow adds every physical row one logical row wraps into, so a cell that took
// two rows leaves the cells beside it padded rather than shifted.
func (t *table) appendRow(dst []row, cells []text.Line, widths []int) []row {
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
		out = append(out, text.Span{Text: fillTo(divider, width), Style: t.rule})
	}
	return out
}

// fillTo covers exactly width columns with a glyph, padding what is left of the
// column rather than stopping short of it.
//
// A divider is configuration: it may be two characters, or one a terminal draws two
// columns wide. Counting repetitions rather than columns made such a rule overrun its
// own column and push the rest of the row along. Counting columns and stopping did
// the opposite for a column no whole number of glyphs fits: the rule was short, and
// every separator after it moved left of the one on the rows it separates.
//
// The remainder is blank because half a glyph is not one, and it is still the
// column's.
func fillTo(glyph string, width int) string {
	unit := text.Width(glyph)
	if width <= 0 || unit <= 0 {
		return ""
	}
	var filled strings.Builder
	filled.Grow(width)
	at := 0
	for ; at+unit <= width; at += unit {
		filled.WriteString(glyph)
	}
	filled.WriteString(strings.Repeat(" ", width-at))
	return filled.String()
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

func blank(n int) text.Span {
	if n <= 0 {
		return text.Span{}
	}
	return text.Span{Text: strings.Repeat(" ", n)}
}
