package headless

import (
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/layout"
	"github.com/Tangerg/oolong/core/text"
)

// editorRow is one visual row: which logical line it came from, and the slice of
// that line it shows.
type editorRow struct {
	line       int
	start, end int
	// joined marks a row that continues the line above rather than starting one.
	joined bool
}

// editorLayout is the wrap of an editor's text at one width.
//
// The editor wraps its own text rather than asking the text package to, because it
// needs the byte offset each row starts at: the cursor lives at an offset, and
// moving it down the screen means finding the offset that sits under it. Measuring
// with one wrap and drawing with another is how a cursor ends up a column away from
// the character it is on.
type editorLayout struct {
	rows  []editorRow
	width int
	stale bool
}

// rowsFor lays the lines out at a width, reusing the last layout when nothing that
// matters has changed.
func (l *editorLayout) rowsFor(lines []string, width int) []editorRow {
	if !l.stale && l.width == width && l.rows != nil {
		return l.rows
	}
	l.rows = l.rows[:0]
	for i, line := range lines {
		l.wrapLine(i, line, width)
	}
	l.width, l.stale = width, false
	return l.rows
}

// wrapLine appends the rows one logical line occupies.
//
// It breaks at spaces when it can and between clusters when it cannot, which is what
// the text package does, so a field and the prose beside it break in the same
// places. A line with nothing in it still gets a row: a blank line in a composer is
// a blank line on screen.
func (l *editorLayout) wrapLine(index int, line string, width int) {
	if width <= 0 || line == "" {
		l.rows = append(l.rows, editorRow{line: index, start: 0, end: len(line)})
		return
	}

	start := 0
	column := 0
	// lastBreak is where the row could end instead, and lastBreakColumn what it
	// would be worth: the offset after the most recent space.
	lastBreak := -1
	for at, cluster := range text.Clusters(line) {
		step := grid.ClusterWidth(cluster)
		if cluster == "\t" {
			step = text.TabStop - column%text.TabStop
		}
		if column+step > width && at > start {
			end := at
			if lastBreak > start {
				end = lastBreak
			}
			l.rows = append(l.rows, editorRow{
				line: index, start: start, end: end, joined: start > 0,
			})
			start = end
			column = text.ColumnOf(line[start:at], at-start)
		}
		if cluster == " " {
			lastBreak = at + len(cluster)
		}
		column += step
	}
	l.rows = append(l.rows, editorRow{line: index, start: start, end: len(line), joined: start > 0})
}

// lastOfLine reports whether the row at i is the final row of its logical line.
func (l *editorLayout) lastOfLine(i int) bool {
	return i == len(l.rows)-1 || l.rows[i+1].line != l.rows[i].line
}

// rowAt is the visual row the cursor is on at a width, and the column within it.
//
// The one position with two answers is where the width broke a line: the offset after
// its last character and the offset before the next row's first are one offset with two
// places on screen. A cursor that arrived by moving through the text belongs to the
// second, and one that arrived by being clicked past the end of a row belongs to the
// first. Nothing in the offset says which, so the field remembers — see
// [Editor.prefersRowEnd].
func (e *Editor) rowAt(width int) (row, column int) {
	rows := e.rows(width)
	atEnd := e.prefersRowEnd()
	for i, r := range rows {
		if r.line != e.line {
			continue
		}
		// The end of a row is the start of the next, so a cursor there belongs to the
		// next row — except on the last row of a line, where there is no next and the
		// cursor sits after the final character, and except when it was put there by a
		// click on this row.
		if e.col < r.end || (e.col == r.end && (atEnd || e.layout.lastOfLine(i))) {
			return i, text.ColumnOf(e.lines[e.line][r.start:r.end], e.col-r.start)
		}
	}
	if len(rows) == 0 {
		return 0, 0
	}
	return len(rows) - 1, 0
}

// offsetIn is the cursor position that sits at a column of a visual row.
func (e *Editor) offsetIn(width, row, column int) (line, col int) {
	rows := e.rows(width)
	if row < 0 || row >= len(rows) {
		return 0, 0
	}
	r := rows[row]
	segment := e.lines[r.line][r.start:r.end]
	return r.line, r.start + text.OffsetAt(segment, column)
}

// MoveUp moves the cursor up one visual row, keeping the column it started from.
func (e *Editor) MoveUp() { e.moveRow(-1) }

// MoveDown moves the cursor down one visual row.
func (e *Editor) MoveDown() { e.moveRow(1) }

// moveRow moves the cursor by visual rows.
//
// The column the cursor was in is remembered across the whole run of movement, so
// travelling down through a short line and out the other side comes back to where it
// went in. Recomputing it each step would drag the cursor left and leave it there.
func (e *Editor) moveRow(delta int) {
	e.ensure()
	e.endTyping()
	width := e.presentation.Value().width
	if width <= 0 {
		// Nothing has been drawn yet, so there are no visual rows to move through.
		// Logical lines are the best available answer, but the horizontal coordinate
		// is still a terminal column rather than a byte offset. Copying the byte offset
		// to a UTF-8 line can put the cursor in the middle of a rune.
		column := text.ColumnOf(e.lines[e.line], e.col)
		if e.wantColumn >= 0 {
			column = e.wantColumn
		}
		target := min(max(e.line+delta, 0), len(e.lines)-1)
		if target == e.line {
			return
		}
		e.line = target
		e.col = e.snapElementBoundary(target, text.OffsetAt(e.lines[target], column), true)
		e.wantColumn = column
		return
	}
	row, column := e.rowAt(width)
	if e.wantColumn >= 0 {
		column = e.wantColumn
	}
	target := row + delta
	if target < 0 || target >= len(e.rows(width)) {
		return
	}
	e.line, e.col = e.offsetIn(width, target, column)
	e.col = e.snapElementBoundary(e.line, e.col, true)
	e.wantColumn = column
}

// Measure is how many rows the field needs at a width, within its cap.
func (e *Editor) Measure(width int) int {
	e.ensure()
	width = e.textWidth(width)
	if e.oneLine() {
		return 1
	}
	rows := len(e.rows(width))
	if e.MaxRows > 0 {
		return min(rows, e.MaxRows)
	}
	return rows
}

// Draw paints the field and places the cursor.
func (e *Editor) Draw(frame Frame) {
	e.DrawWith(frame, e.Look)
}

// DrawWith paints one projection with look without changing the editor's configured
// appearance.
//
// Appearance components use this when an editor participates in a larger theme. The
// editor remains the single owner of its text, cursor and input configuration; drawing
// it through another look does not make that look its configuration.
func (e *Editor) DrawWith(frame Frame, look Look) {
	e.drawWith(frame, look, &e.presentation)
}

// drawWith separates the state being projected from the routing geometry that will
// receive the next event. A wrapper may render a temporary projection while staging
// its geometry into the durable editor it owns.
func (e *Editor) drawWith(frame Frame, look Look, presented *Snapshot[editorPresentation]) {
	v := frame.View
	total, height := v.Size()
	if total <= 0 || height <= 0 {
		presented.Stage(frame, editorPresentation{})
		return
	}
	e.ensure()
	gutter := min(e.gutterWidth(), total)
	width := layout.Remaining(total, gutter)
	gutterView := v.Sub(grid.Rect(0, 0, gutter, height))
	v = v.Sub(grid.Rect(gutter, 0, width, height))
	if width <= 0 {
		presented.Stage(frame, editorPresentation{})
		return
	}
	if e.oneLine() {
		presentation := e.drawOneLine(v, gutterView, look, width, gutter)
		presented.Stage(frame, presentation)
		return
	}
	presentation := e.drawMultiline(frame, v, gutterView, look, width, height, gutter)
	presented.Stage(frame, presentation)
}

func (e *Editor) drawOneLine(
	view, gutterView grid.View,
	look Look,
	width, gutter int,
) editorPresentation {
	left := e.lineOffset(width)
	e.drawGutter(gutterView, e.rows(width))
	e.drawLine(view, left, look)
	return editorPresentation{width: width, gutter: gutter, left: left}
}

func (e *Editor) drawMultiline(
	frame Frame,
	view, gutterView grid.View,
	look Look,
	width, height, gutter int,
) editorPresentation {
	rows := e.rows(width)
	cursorRow, cursorColumn := e.rowAt(width)

	// The field scrolls only when it is taller than its box, and then only as far as
	// it must to keep the cursor visible: a field that jumped to the end would lose
	// the line the user is typing on.
	scroll := e.scroll.Stage(frame, len(rows), height)
	scroll.Reveal(cursorRow)
	first := scroll.Offset()
	presentation := editorPresentation{width: width, gutter: gutter, first: first}
	last := min(layout.Sum(first, height), len(rows))
	e.drawGutter(gutterView, rows[first:last])

	if e.Empty() && e.Placeholder != "" {
		view.Text(0, 0, text.Truncate(e.Placeholder, width, "…"), look.Subtle)
		e.placeCursor(view, 0, 0)
		return presentation
	}

	for y := range height {
		index := layout.Sum(first, y)
		if index >= len(rows) {
			break
		}
		r := rows[index]
		text.Of(e.lines[r.line][r.start:r.end], look.Text).Draw(view, 0, y)
	}
	// The selection is laid over the text rather than drawn into it, so a run that
	// crosses a style boundary keeps whatever was underneath — and so that the rows
	// above did not have to be told which of them was selected.
	for _, span := range e.spansOfSelection(width) {
		y := span.Row - first
		if y < 0 || y >= height {
			continue
		}
		end := min(layout.Sum(span.Col, span.Width), width)
		for x := max(span.Col, 0); x < end; x++ {
			view.MergeStyle(x, y, look.Selection)
		}
	}
	if y := cursorRow - first; y >= 0 && y < height {
		e.placeCursor(view, cursorColumn, y)
	}
	return presentation
}

func (e *Editor) gutterWidth() int {
	if e.Gutter == nil {
		return 0
	}
	return max(e.Gutter.Width(len(e.lines)), 0)
}

func (e *Editor) textWidth(total int) int {
	return layout.Remaining(total, e.gutterWidth())
}

func (e *Editor) drawGutter(view grid.View, rows []editorRow) {
	if e.Gutter == nil {
		return
	}
	out := make([]text.Row, len(rows))
	for i, row := range rows {
		shown := e.lines[row.line][row.start:row.end]
		if e.mask != "" {
			// A masked field must not disclose its value to an appearance callback.
			shown = e.shown()
		}
		out[i] = text.Row{
			Text: shown,
			Line: row.line + 1, Joined: row.joined,
		}
	}
	e.Gutter.Draw(view, out)
}

// Focus takes the keyboard, or gives it up. A field without it draws no cursor.
//
// A frame has one cursor and the terminal draws it, so two fields both asking for it
// is not two cursors: it is one, wherever the last of them happened to draw. This is
// how the question is settled — see [Focusable], and note that a field nobody has
// told anything believes it has the keyboard, which is what makes a lone field work.
func (e *Editor) Focus(has bool) {
	if !has {
		e.matcher.Clear()
		e.endTyping()
	}
	e.blurred = !has
}

// placeCursor asks for the terminal's cursor only when this field has the keyboard.
func (e *Editor) placeCursor(v grid.View, x, y int) {
	if e.blurred {
		return
	}
	v.PlaceCursor(x, y, e.CursorStyle)
}

// Scroll exposes the field's position, for a scrollbar beside a tall field.
func (e *Editor) Scroll() *Scroll { return &e.scroll }
