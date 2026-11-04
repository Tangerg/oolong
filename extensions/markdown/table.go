package markdown

import (
	east "github.com/yuin/goldmark/extension/ast"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/text"
)

// tableColumnFloor is the smallest column that still reads as a column. A table
// that cannot afford this much room per non-empty column becomes records instead
// of preserving a grid whose cells are mostly vertical fragments.
const tableColumnFloor = 4

type table struct {
	rows   [][]text.Line
	aligns []layout.Align
	header bool

	separator string
	divider   string
	rail      grid.Style
	rule      grid.Style
}

func columnAlignments(in []east.Alignment) []layout.Align {
	out := make([]layout.Align, len(in))
	for i, alignment := range in {
		switch alignment {
		case east.AlignCenter:
			out[i] = layout.Center
		case east.AlignRight:
			out[i] = layout.End
		case east.AlignLeft, east.AlignNone:
			out[i] = layout.Start
		}
	}
	return out
}

func (t *table) appendRows(dst []text.Wrapped, room int) []text.Wrapped {
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
	measures := t.measureColumns()
	if len(measures) == 0 {
		return nil, true
	}
	budget := room - t.separatorWidth(len(measures))
	widths := make([]int, len(measures))
	used := 0
	for i, measure := range measures {
		widths[i] = min(measure.natural, max(tableColumnFloor, measure.indivisible))
		used = layout.Sum(used, widths[i])
	}
	if used > budget {
		return nil, false
	}
	t.growColumns(widths, measures, budget-used)
	return widths, true
}

type columnMeasure struct{ natural, indivisible int }

func (t *table) measureColumns() []columnMeasure {
	measures := make([]columnMeasure, t.columns())
	for _, row := range t.rows {
		for column, cell := range row {
			measure := &measures[column]
			measure.natural = max(measure.natural, cell.Width())
			// Wrapping preserves an oversized grapheme; a narrower grid column
			// would let that atom displace or hide the fields beside it.
			for _, cluster := range text.Clusters(cell.String()) {
				measure.indivisible = max(measure.indivisible, grid.ClusterWidth(cluster))
			}
		}
	}
	return measures
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
func (t *table) growColumns(widths []int, measures []columnMeasure, remaining int) {
	pending := make([]int, 0, len(widths))
	for i := range widths {
		if widths[i] < measures[i].natural {
			pending = append(pending, i)
		}
	}
	for remaining > 0 && len(pending) > 0 {
		share := remaining / len(pending)
		if share == 0 {
			break
		}
		rest, left := settleColumns(widths, measures, pending, share, remaining)
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

func settleColumns(widths []int, measures []columnMeasure, pending []int, share, remaining int) ([]int, int) {
	still := pending[:0]
	for _, column := range pending {
		need := measures[column].natural - widths[column]
		if need > share {
			still = append(still, column)
			continue
		}
		widths[column] += need
		remaining -= need
	}
	return still, remaining
}

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

func (t *table) appendGrid(dst []text.Wrapped, widths []int) []text.Wrapped {
	for rowIndex, cells := range t.rows {
		dst = t.appendRow(dst, cells, widths)
		if rowIndex == 0 && t.header {
			dst = append(dst, text.Wrapped{Line: t.ruleLine(widths)})
		}
	}
	return dst
}

// appendRow adds every physical row one logical row wraps into, so a cell that took
// two rows leaves the cells beside it padded rather than shifted.
func (t *table) appendRow(dst []text.Wrapped, cells []text.Line, widths []int) []text.Wrapped {
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
			line = append(line, cell.Pad(width, t.alignment(column))...)
		}
		dst = append(dst, text.Wrapped{Line: line})
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
		out = append(out, text.Span{Text: text.Fill(divider, width), Style: t.rule})
	}
	return out
}

// appendRecords uses the heading cells as field names. No labels are invented for
// a table without a heading; each cell simply gets a line of its own. A blank row
// keeps adjacent records distinct without introducing product-specific furniture.
func (t *table) appendRecords(dst []text.Wrapped, room int) []text.Wrapped {
	var keys []text.Line
	records := t.rows
	if t.header && len(t.rows) > 1 {
		keys, records = t.rows[0], t.rows[1:]
	}
	for recordIndex, record := range records {
		if recordIndex > 0 {
			dst = append(dst, text.Wrapped{})
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
			dst = append(dst, line.Wrap(room)...)
		}
	}
	return dst
}

func (t *table) alignment(column int) layout.Align {
	if column < 0 || column >= len(t.aligns) {
		return layout.Start
	}
	return t.aligns[column]
}
