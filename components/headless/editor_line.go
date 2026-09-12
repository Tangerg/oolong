package headless

import (
	"strings"

	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/layout"
	"github.com/Tangerg/oolong/core/text"
)

// A field that holds one line does not wrap: it slides sideways to keep the cursor in
// view, the way every one-line field in every terminal does. Wrapping is what the rest
// of this editor is arranged around, so the one place the two differ is here.

// rows is the field's text laid out at a width.
//
// A field holding one line is laid out at no width at all, which is how the wrap is
// told not to break anything: the line is one row however long it is, and what is off
// the side of the box is off the side of the box. Everything that reads rows — moving
// the cursor, finding a click, drawing a selection — then agrees, because there is one
// layout and they all ask it.
func (e *Editor) rows(width int) []editorRow {
	if e.oneLine() {
		width = 0
	}
	return e.layout.rowsFor(e.lines, width)
}

// editorLineView is read-only rendering input, shared by Editor and controlled Text.
// It owns no editor history, input matcher, mutable document or frame transaction.
type editorLineView struct {
	value, placeholder, mask string
	cursor, anchor, left     int
	selecting, blurred       bool
	gutter                   RowGutter
	cursorStyle              grid.CursorStyle
}

func (e *Editor) lineView(value string) editorLineView {
	view := editorLineView{
		value: value, placeholder: e.Placeholder, mask: e.mask,
		cursor: e.col, anchor: e.anchor.Col, selecting: e.selecting,
		blurred: e.blurred, left: e.presentation.Value().left,
		gutter: e.Gutter, cursorStyle: e.CursorStyle,
	}
	if value != e.Text() {
		view.cursor, view.selecting = len(value), false
	}
	return view
}

func (e editorLineView) shown() string {
	if e.mask == "" {
		return e.value
	}
	var shown strings.Builder
	for range text.Clusters(e.value) {
		shown.WriteString(e.mask)
	}
	return shown.String()
}

func (e editorLineView) shownAt(column int) int {
	if e.mask == "" {
		return column
	}
	at := 0
	for offset := range text.Clusters(e.value) {
		if offset >= column {
			break
		}
		at += len(e.mask)
	}
	return at
}

func (e editorLineView) draw(frame Frame, look Look, presented *Snapshot[editorPresentation]) {
	total, height := frame.Size()
	gutter := 0
	if e.gutter != nil {
		gutter = min(max(e.gutter.Width(1), 0), max(total, 0))
	}
	width := layout.Remaining(total, gutter)
	if width <= 0 || height <= 0 {
		presented.Stage(frame, editorPresentation{})
		return
	}
	shown := e.shown()
	cursor := text.ColumnOf(shown, e.shownAt(e.cursor))
	left := min(cursor, e.left)
	if cursor > layout.Sum(left, width-1) {
		left = layout.Remaining(cursor, width-1)
	}
	left = max(min(left, layout.Remaining(text.Width(shown), width-1)), 0)
	if e.gutter != nil {
		e.gutter.Draw(frame.Sub(grid.Rect(0, 0, gutter, height)).View, []text.Row{{Text: shown, Line: 1}})
	}
	view := frame.Sub(grid.Rect(gutter, 0, width, height)).View
	if e.value == "" && e.placeholder != "" {
		view.Text(0, 0, text.Truncate(e.placeholder, width, "…"), look.Subtle)
	} else {
		text.Of(shown, look.Text).Draw(view, -left, 0)
		if e.selecting {
			from := text.ColumnOf(shown, e.shownAt(min(e.anchor, e.cursor))) - left
			to := text.ColumnOf(shown, e.shownAt(max(e.anchor, e.cursor))) - left
			for x := max(from, 0); x < min(to, width); x++ {
				view.MergeStyle(x, 0, look.Selection)
			}
		}
	}
	if !e.blurred {
		view.PlaceCursor(cursor-left, 0, e.cursorStyle)
	}
	presented.Stage(frame, editorPresentation{width: width, gutter: gutter, left: left})
}

// shown is the text as it is drawn: the line itself, or the mask once per cluster for
// a field holding something the screen should not show.
func (e *Editor) shown() string {
	e.ensure()
	return e.lineView(e.lines[0]).shown()
}

// lineAt is an offset into what is drawn as one into the line.
func (e *Editor) lineAt(at int) int {
	if e.mask == "" {
		return at
	}
	line := e.lines[0]
	if at <= 0 {
		return 0
	}
	want := at / len(e.mask)
	seen := 0
	for offset := range text.Clusters(line) {
		if seen == want {
			return offset
		}
		seen++
	}
	return len(line)
}

// atLine is where a point lands in a field that holds one line.
func (e *Editor) atLine(x int) Caret {
	col := e.lineAt(text.OffsetAt(e.shown(), layout.Translate(x, e.presentation.Value().left)))
	return Caret{Col: e.snapElement(0, col, true)}
}
