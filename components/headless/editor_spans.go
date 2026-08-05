package headless

import (
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/text"
)

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

// At is the position in the text under a point in the field's box, and whether the
// point is in the text at all.
//
// The point is in the field's own coordinates, which is what a widget is handed. The
// answer accounts for the field having scrolled, because the field is what knows it.
//
// It reads the same rows the cursor is placed from and the selection is painted from,
// which is the only way a click can land where the reader thinks they clicked: three
// walks over three wraps agree until the text is interesting, and then they do not.
func (e *Editor) At(x, y, width int) (Caret, bool) {
	if width <= 0 || x < 0 || y < 0 {
		return Caret{}, false
	}
	e.ensure()
	rows := e.layout.rowsFor(e.lines, width)
	index := e.scroll.Offset() + y
	if index >= len(rows) {
		// Below the text. The end is where a click there means, the way it does in
		// every editor: a reader clicking past the last line means the last line.
		//
		// There is always a row to be past, so nothing here checks. A field holds at
		// least one line even when it is empty, and every line gets a row even when
		// it has nothing on it — a blank line in a composer is a blank line on screen.
		last := rows[len(rows)-1]
		return Caret{Line: last.line, Col: last.end}, true
	}
	r := rows[index]
	line := e.lines[r.line]
	// A click past the end of the row lands at the end of its text. Where the width
	// broke a line, that offset and the offset before the next row's first character
	// are one offset with two places on screen, and this editor draws it in the second
	// — so clicking past the end of a wrapped row shows the caret at the start of the
	// next one. Telling them apart takes an affinity bit on the caret, which is a
	// larger change than this and is on the list.
	col := r.start + text.OffsetAt(line[r.start:r.end], x)
	// Out of any element it lands in, forwards, so a click in the middle of a chip
	// puts the cursor after it rather than inside — the same rule moving does.
	return Caret{Line: r.line, Col: e.snapElement(r.line, col, true)}, true
}

// HandleMouse answers a mouse event, reporting whether it consumed it.
//
// A press puts the cursor where it was pressed and starts a selection there; a drag
// with the button held moves the far end; a release ends it. That is what a text field
// does everywhere, and until now this one did none of it — a field could be typed into
// and not clicked into.
//
// It is separate from [Editor.Handle] because a mouse position means nothing without
// knowing where the field was drawn. Handle is given an event and no geometry; this is
// given the point already translated into the field's own box, which whatever drew it
// is the only thing that can do.
func (e *Editor) HandleMouse(ev input.Mouse, width int) bool {
	switch ev.Action {
	case input.MouseDown:
		if ev.Button != input.ButtonLeft {
			return false
		}
		at, ok := e.At(ev.Pos.X, ev.Pos.Y, width)
		if !ok {
			return false
		}
		e.SelectNone()
		e.endTyping()
		e.line, e.col, e.wantColumn = at.Line, at.Col, -1
		e.anchor, e.selecting = at, true
		return true
	case input.MouseDrag:
		if !e.selecting {
			return false
		}
		at, ok := e.At(ev.Pos.X, ev.Pos.Y, width)
		if !ok {
			return false
		}
		e.line, e.col, e.wantColumn = at.Line, at.Col, -1
		return true
	case input.MouseUp:
		if !e.selecting {
			return false
		}
		// An anchor still on the cursor is a click and not a selection, which
		// [Editor.Selection] already reports as nothing selected. Nothing to undo.
		return true
	default:
		return false
	}
}
