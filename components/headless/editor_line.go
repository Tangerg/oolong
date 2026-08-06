package headless

import (
	"strings"

	"github.com/Tangerg/oolong/core/grid"
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

// drawLine paints a field that holds one line, and places the cursor.
func (e *Editor) drawLine(v grid.View) {
	width, _ := v.Size()
	if e.Empty() && e.Placeholder != "" {
		v.Text(0, 0, text.Truncate(e.Placeholder, width, "…"), e.Look.Subtle)
		e.placeCursor(v, 0, 0)
		return
	}

	shown := e.shown()
	cursor := text.ColumnOf(shown, e.shownAt(e.col))
	e.slide(cursor, text.Width(shown), width)

	text.Of(shown, e.Look.Text).Draw(v, -e.left, 0)
	if start, end, ok := e.Selection(); ok {
		from := text.ColumnOf(shown, e.shownAt(start.Col)) - e.left
		to := text.ColumnOf(shown, e.shownAt(end.Col)) - e.left
		for x := max(from, 0); x < min(to, width); x++ {
			if c := v.CellAt(x, 0); c != nil {
				c.Style = c.Style.Merge(e.Look.Selection)
			}
		}
	}
	e.placeCursor(v, cursor-e.left, 0)
}

// slide moves the window the least it can to keep the cursor in it, and no further
// than there is text to show.
//
// The second half is what stops a field from being left showing its end after a
// deletion, with blank columns to the right of text that would have fitted. The cursor
// may sit one column past the last character, which is why the window has to reach one
// column further than the text does.
func (e *Editor) slide(cursor, total, width int) {
	if width <= 0 {
		e.left = 0
		return
	}
	if cursor < e.left {
		e.left = cursor
	}
	if cursor > e.left+width-1 {
		e.left = cursor - width + 1
	}
	e.left = min(e.left, max(total-width+1, 0))
	e.left = max(e.left, 0)
}

// shown is the text as it is drawn: the line itself, or the mask once per cluster for
// a field holding something the screen should not show.
func (e *Editor) shown() string {
	e.ensure()
	line := e.lines[0]
	if e.Mask == "" {
		return line
	}
	var b strings.Builder
	for range text.Clusters(line) {
		b.WriteString(e.Mask)
	}
	return b.String()
}

// shownAt is an offset into the line as one into what is drawn.
//
// A mask is one cluster per cluster, so the two are the same count of clusters and a
// different count of bytes. Nothing else in the field has to know that: the cursor, the
// selection and a click all pass through here and through [Editor.lineAt].
func (e *Editor) shownAt(col int) int {
	if e.Mask == "" {
		return col
	}
	at := 0
	for offset := range text.Clusters(e.lines[0]) {
		if offset >= col {
			break
		}
		at += len(e.Mask)
	}
	return at
}

// lineAt is an offset into what is drawn as one into the line.
func (e *Editor) lineAt(at int) int {
	if e.Mask == "" {
		return at
	}
	line := e.lines[0]
	if at <= 0 {
		return 0
	}
	want := at / len(e.Mask)
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
	col := e.lineAt(text.OffsetAt(e.shown(), x+e.left))
	return Caret{Col: e.snapElement(0, col, true)}
}
