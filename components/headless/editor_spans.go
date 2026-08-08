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
// is a question only the field can answer, and only at a width. Width is the whole
// field, including any [Editor.Gutter], and returned columns use that same coordinate
// space.
//
// It reads the same rows the cursor is placed with. That is the whole point of it
// being here rather than being worked out by whatever draws: a selection painted from
// one wrap and a cursor placed from another disagree about where the text is, and the
// disagreement shows up exactly when the text is interesting — a long word, a wide
// character, a line that just fits.
func (e *Editor) Spans(from, to Caret, width int) []RowSpan {
	gutter := e.gutterWidth()
	out := e.spans(from, to, max(width-gutter, 0))
	for i := range out {
		out[i].Col += gutter
	}
	return out
}

func (e *Editor) spans(from, to Caret, width int) []RowSpan {
	if width <= 0 {
		return nil
	}
	e.ensure()
	if to.Before(from) {
		from, to = to, from
	}
	rows := e.rows(width)

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

func (e *Editor) spansOfSelection(width int) []RowSpan {
	start, end, ok := e.Selection()
	if !ok {
		return nil
	}
	return e.spans(start, end, width)
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
	gutter := e.gutterWidth()
	if x < gutter {
		return Caret{}, false
	}
	return e.at(x-gutter, y, max(width-gutter, 0))
}

func (e *Editor) at(x, y, width int) (Caret, bool) {
	if width <= 0 || x < 0 || y < 0 {
		return Caret{}, false
	}
	e.ensure()
	if e.oneLine() {
		return e.atLine(x), true
	}
	rows := e.rows(width)
	presented := e.presentation.Value()
	first := 0
	if presented.width == width {
		first = presented.first
	}
	index := first + y
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
	col := r.start + text.OffsetAt(line[r.start:r.end], x)
	// Out of any element it lands in, forwards, so a click in the middle of a chip
	// puts the cursor after it rather than inside — the same rule moving does.
	return Caret{Line: r.line, Col: e.snapElement(r.line, col, true)}, true
}

// handleMouse answers a mouse event at a width, reporting whether it consumed it.
//
// A press puts the cursor where it was pressed and starts a selection there; a drag
// with the button held moves the far end; a release ends it. That is what a text field
// does everywhere.
//
// The width is a parameter rather than a field because this is where the arithmetic
// lives, not because a caller gets to choose one. [Editor.Handle] supplies the width
// the editor last drew at, which is the only width a pointer event can be about: a
// press is aimed at what is on the screen. Routing one against a width that was never
// presented would answer a question nobody asked.
func (e *Editor) handleMouse(ev input.Mouse, presented editorPresentation) bool {
	if presented.width <= 0 || ev.Pos.X < presented.gutter {
		return false
	}
	ev.Pos.X -= presented.gutter
	switch ev.Action {
	case input.MouseDown:
		if ev.Button != input.ButtonLeft {
			return false
		}
		at, ok := e.at(ev.Pos.X, ev.Pos.Y, presented.width)
		if !ok {
			return false
		}
		e.SelectNone()
		e.endTyping()
		e.line, e.col, e.wantColumn = at.Line, at.Col, -1
		e.anchor, e.selecting = at, true
		// A click that landed past the end of a wrapped row means that row, not the
		// start of the next. It is the only way a cursor comes to be at a soft break
		// and belong to the earlier side.
		e.rowEnd, e.rowEndSet = at, e.pastRowEnd(ev.Pos.X, ev.Pos.Y, presented.width)
		return true
	case input.MouseDrag:
		if !e.selecting {
			return false
		}
		at, ok := e.at(ev.Pos.X, ev.Pos.Y, presented.width)
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

// pastRowEnd reports whether a point is beyond the text of a row that the width broke,
// which is the only place a cursor can belong to the earlier side of a break.
func (e *Editor) pastRowEnd(x, y, width int) bool {
	rows := e.rows(width)
	presented := e.presentation.Value()
	first := 0
	if presented.width == width {
		first = presented.first
	}
	index := first + y
	if index < 0 || index >= len(rows) || e.layout.lastOfLine(index) {
		return false
	}
	r := rows[index]
	return x >= text.Width(e.lines[r.line][r.start:r.end])
}

// prefersRowEnd reports whether the cursor should be drawn at the end of a wrapped row
// rather than at the start of the next. See [Editor.rowEnd].
func (e *Editor) prefersRowEnd() bool {
	return e.rowEndSet && e.rowEnd == Caret{Line: e.line, Col: e.col}
}
