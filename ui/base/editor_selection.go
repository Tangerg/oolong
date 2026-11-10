package base

import (
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/text"
)

// Anchor preserves an existing selection's origin so repeated extension actions
// can move the caret without moving the start of the gesture.
func (e *Editor) Anchor() {
	e.beginViewUpdate()
	defer e.endViewUpdate()
	e.breakContinuation()
	if e.selection == editorSelectionNone {
		e.anchor, e.selection = Caret{Line: e.line, Byte: e.byteOffset}, editorSelecting
	}
}

func (e *Editor) SelectNone() {
	e.beginViewUpdate()
	defer e.endViewUpdate()
	e.breakContinuation()
	e.selection = editorSelectionNone
}

func (e *Editor) SelectAll() {
	e.beginViewUpdate()
	defer e.endViewUpdate()
	defer e.revealCursor()
	e.ensure()
	e.endTyping()
	e.anchor, e.selection = Caret{}, editorSelecting
	e.line = len(e.document().lines) - 1
	e.byteOffset = len(e.document().lines[e.line])
	e.desiredColumn = editorColumn{}
}

// Selection is the selected range in reading order, and whether there is one.
//
// It reports false for a selection of nothing, which is what a shift-arrow pressed
// and then taken back leaves: an anchor at the cursor is not a selection, and
// treating it as one would make a copy put an empty string on the clipboard.
func (e *Editor) Selection() (start, end Caret, ok bool) {
	if e.selection == editorSelectionNone {
		return Caret{}, Caret{}, false
	}
	cursor := Caret{Line: e.line, Byte: e.byteOffset}
	if cursor == e.anchor {
		return Caret{}, Caret{}, false
	}
	if cursor.Before(e.anchor) {
		return cursor, e.anchor, true
	}
	return e.anchor, cursor, true
}

// Selected is the selected text, or empty when nothing is selected.
func (e *Editor) Selected() string {
	start, end, ok := e.Selection()
	if !ok {
		return ""
	}
	return e.document().textBetween(start, end)
}

func (e *Editor) DeleteSelection() bool {
	e.beginViewUpdate()
	defer e.endViewUpdate()
	start, end, ok := e.Selection()
	if !ok {
		return false
	}
	e.document().replace(e, editReplacement{start: start, end: end}, e.finishReplacement)
	return true
}

// Copy sends the selection to the clipboard. Masked sessions never export their
// underlying text. Like Paste and Cut, clipboard access is an owner-side effect.
func (e *Editor) Copy() bool {
	e.beginViewUpdate()
	defer e.endViewUpdate()
	start, end, ok := e.Selection()
	return ok && e.copyRange(start, end)
}

func (e *Editor) copyRange(start, end Caret) bool {
	return e.Mask() == "" && e.clipboard != nil && e.clipboard.Copy(e.document().textBetween(start, end))
}

// Cut copies and removes the selection only after clipboard acceptance. Clipboard
// callbacks that change content or the selected range revoke the removal, so an
// older copy cannot authorize deleting newer text.
func (e *Editor) Cut() bool {
	e.beginViewUpdate()
	defer e.endViewUpdate()
	e.endTyping()
	start, end, selected := e.Selection()
	if !selected {
		return false
	}
	revision := e.document().revision
	if !e.copyRange(start, end) {
		return false
	}
	currentStart, currentEnd, current := e.Selection()
	if e.document().revision != revision || !current || currentStart != start || currentEnd != end {
		return false
	}
	return e.DeleteSelection()
}

// Spans returns visual column runs covered by two carets. Width and columns include
// [Editor.Gutter]. Cursor placement and selection painting share the same wrap so
// graphemes cannot acquire different screen positions.
func (e *Editor) Spans(from, to Caret, width int) []RowSpan {
	gutter := e.gutterWidth()
	out := e.spans(from, to, layout.Remaining(width, gutter))
	for i := range out {
		out[i].Col = layout.Sum(out[i].Col, gutter)
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
	if e.Mask() != "" {
		view := e.lineView()
		shown := view.shown()
		if from.Line != 0 || to.Line != 0 {
			return nil
		}
		start := text.ColumnOf(shown, view.shownAt(from.Byte))
		end := text.ColumnOf(shown, view.shownAt(to.Byte))
		if end <= start {
			return nil
		}
		return []RowSpan{{Row: 0, Col: start, Width: end - start}}
	}
	rows := e.rows(width)

	var out []RowSpan
	for i, r := range rows {
		if r.line < from.Line || r.line > to.Line {
			continue
		}
		lo, hi := r.start, r.end
		if r.line == from.Line {
			lo = max(lo, from.Byte)
		}
		if r.line == to.Line {
			hi = min(hi, to.Byte)
		}
		if lo >= hi {
			continue
		}
		line := e.document().lines[r.line]
		column := text.ColumnOf(line[r.start:r.end], lo-r.start)
		end := text.ColumnOf(line[r.start:r.end], hi-r.start)
		out = append(out, RowSpan{Row: i, Col: column, Width: end - column})
	}
	return out
}

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

// RowSpan is a visual column run. Row is relative to the whole wrapped text so
// scrolling does not change its coordinates.
type RowSpan struct {
	Row        int
	Col, Width int
}

// editorSelection is whether the anchor marks a selection and whether a pointer
// drag is extending it; a drag without a selection to extend is not representable.
type editorSelection uint8

const (
	editorSelectionNone editorSelection = iota
	editorSelecting
	editorDragging
)

// endDrag keeps the selection a drag extended and reports whether one was running.
func (e *Editor) endDrag() bool {
	if e.selection != editorDragging {
		return false
	}
	e.selection = editorSelecting
	return true
}
