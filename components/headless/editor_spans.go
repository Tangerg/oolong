package headless

import "github.com/Tangerg/oolong/core/text"

// RowSpan is a run of columns on one visual row of a field.
//
// The row is counted from the top of the whole wrapped text and not from the top of
// the box, because the field scrolls: a caller that wanted rows on screen would have
// to be told the scroll position to make sense of them, and the field already knows
// it.
type RowSpan struct {
	Row        int
	Col, Width int
}

// Spans is the runs of columns that the text between two carets covers, one per
// visual row it crosses.
//
// A range in a wrapped field is not a rectangle and is rarely one run. It starts part
// way along a row, covers whole rows, and ends part way along another — and where the
// rows begin and end is decided by the wrap, which is decided by the width. So this
// is a question only the field can answer, and only at a width.
//
// It reads the same rows the cursor is placed with. That is the whole point of it
// being here rather than being worked out by whatever draws: a selection painted from
// one wrap and a cursor placed from another disagree about where the text is, and the
// disagreement shows up exactly when the text is interesting — a long word, a wide
// character, a line that just fits.
func (e *Editor) Spans(from, to Caret, width int) []RowSpan {
	if width <= 0 {
		return nil
	}
	e.ensure()
	if to.Before(from) {
		from, to = to, from
	}
	rows := e.layout.rowsFor(e.lines, width)

	var out []RowSpan
	for i, r := range rows {
		if r.line < from.Line || r.line > to.Line {
			continue
		}
		// The part of this row the range covers, in offsets into the line.
		lo, hi := r.start, r.end
		if r.line == from.Line {
			lo = max(lo, from.Col)
		}
		if r.line == to.Line {
			hi = min(hi, to.Col)
		}
		if lo >= hi {
			continue
		}
		line := e.lines[r.line]
		col := text.ColumnOf(line[r.start:r.end], lo-r.start)
		end := text.ColumnOf(line[r.start:r.end], hi-r.start)
		out = append(out, RowSpan{Row: i, Col: col, Width: end - col})
	}
	return out
}

// SelectionSpans is where the selection is, or nothing when there is none.
func (e *Editor) SelectionSpans(width int) []RowSpan {
	start, end, ok := e.Selection()
	if !ok {
		return nil
	}
	return e.Spans(start, end, width)
}
