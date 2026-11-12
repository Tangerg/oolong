package base

import (
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/core/text"
)

func (e *Editor) HeightForWidth(width int) int {
	width = e.textWidth(width)
	if e.oneLine() {
		return 1 + feedbackRows(e.PasteError())
	}
	rows := len(e.rows(width))
	if e.maxRows > 0 {
		rows = min(rows, e.maxRows)
	}
	return layout.Sum(rows, feedbackRows(e.PasteError()))
}

func (e *Editor) Draw(frame scene.Frame) {
	e.DrawWith(frame, e.look)
}

// DrawWith paints one projection with look without changing the editor's configured
// appearance.
//
// Appearance components use this when an editor participates in a larger theme. The
// editor remains the single owner of its text, cursor and input configuration; drawing
// it through another look does not make that look its configuration.
// It panics unless the editor is mounted in the frame's scene outside read-only projection.
func (e *Editor) DrawWith(frame scene.Frame, look Look) {
	requireProjection(e.element, frame)
	frame = feedbackFrame(frame, e.PasteError(), look)
	presented := &e.presentation
	if e.oneLine() {
		e.lineView().draw(frame, look, presented)
		return
	}
	v := frame.View
	total, height := v.Size()
	if total <= 0 || height <= 0 {
		presented.Stage(frame, editorPresentation{})
		return
	}
	gutter := min(e.gutterWidth(), total)
	width := layout.Remaining(total, gutter)
	gutterView := v.Sub(grid.Area(0, 0, gutter, height))
	v = v.Sub(grid.Area(gutter, 0, width, height))
	if width <= 0 {
		presented.Stage(frame, editorPresentation{})
		return
	}
	presentation := e.drawMultiline(frame, v, gutterView, look, width, height, gutter)
	presented.Stage(frame, presentation)
}

func (e *Editor) drawMultiline(
	frame scene.Frame,
	view, gutterView grid.View,
	look Look,
	width, height, gutter int,
) editorPresentation {
	rows := e.rows(width)
	cursorRow, cursorColumn := e.rowAt(width)

	scroll := e.scroll.Stage(frame, len(rows), height)
	if e.scroll.revealPending(e.cursorReveal) {
		// Wrapping may have changed since navigation. Resolve the logical cursor
		// using this frame, but only while its request has not been superseded.
		scroll.Reveal(cursorRow, cursorRow)
	}
	first := scroll.Offset()
	projection := e.layout.projection
	presentation := editorPresentation{content: e.projectionKey(), width: width, gutter: gutter, first: first, visible: frame.Visible()}
	last := min(layout.Sum(first, height), len(rows))
	e.drawGutter(gutterView, projection, first, last)

	if e.Empty() && e.placeholder != "" {
		text.Of(e.placeholder, look.Subtle).Truncate(width, look.Ellipsis).Draw(view, 0, 0)
		e.placeCursor(view, 0, 0)
		return presentation
	}

	visible := view.Visible()
	projection.Draw(view.Sub(grid.Area(0, -first, width, len(rows))), look.Text)
	// The selection is laid over the text rather than drawn into it, so a run that
	// crosses a style boundary keeps whatever was underneath — and so that the rows
	// above did not have to be told which of them was selected.
	for _, span := range e.spansOfSelection(width) {
		y := span.Row - first
		if y < visible.Min.Y || y >= visible.Max.Y {
			continue
		}
		end := min(layout.Sum(span.Col, span.Width), visible.Max.X)
		for x := max(span.Col, visible.Min.X); x < end; x++ {
			view.MergeStyle(x, y, look.Selection)
		}
	}
	if y := cursorRow - first; y >= 0 && y < height {
		e.placeCursor(view, cursorColumn, y)
	}
	return presentation
}

func (e *Editor) gutterWidth() int {
	if e.gutter == nil {
		return 0
	}
	return readProjection(e.Element(), func() int { return max(e.gutter.Width(max(len(e.document().lines), 1)), 0) })
}

func (e *Editor) textWidth(total int) int {
	return layout.Remaining(total, e.gutterWidth())
}

func (e *Editor) drawGutter(view grid.View, projection *text.Layout, first, last int) {
	if e.gutter == nil {
		return
	}
	rows := make([]text.Row, last-first)
	for i := range rows {
		row, _ := projection.Row(first + i)
		rows[i] = text.Row{Content: row.Content, SourceLine: row.Line + 1, Joined: row.Joined}
	}
	e.gutter.Draw(view, rows)
}

func (e *Editor) placeCursor(v grid.View, x, y int) {
	if !e.element.Focused() {
		return
	}
	v.PlaceCursor(x, y, e.cursorStyle)
}

// revealCursor is the semantic navigation boundary. A draw may refine the visual
// row but never creates a new request; manual scrolling can therefore cancel it.
func (e *Editor) revealCursor() {
	if e.oneLine() {
		return
	}
	e.ensure()
	width := e.geometry.read(&e.presentation).width
	row, _ := e.rowAt(width)
	e.scroll.revealContent(len(e.rows(width)), row, row)
	e.cursorReveal = e.scroll.reveal
}

// Scroll exposes the field's position, for a scrollbar beside a tall field.
// Editing and cursor navigation reveal the cursor once. Manual scrolling remains
// in effect through redraws and resizes until the next navigation or edit.
func (e *Editor) Scroll() *Scroll { return &e.scroll }

// Zero wrap width keeps one-line fields on one visual row for drawing and hit testing.
func (e *Editor) rows(width int) []editorRow {
	if e.oneLine() {
		width = 0
	}
	lines := e.document().lines
	if len(lines) == 0 {
		lines = []string{""}
	}
	return e.layout.rowsFor(lines, width)
}

// A pointer may only address the text and display policy used by its arrival frame.
type editorProjectionKey struct {
	revision   uint64
	singleLine bool
	mask       string
}

func (e *Editor) projectionKey() editorProjectionKey {
	return editorProjectionKey{revision: e.Revision(), singleLine: e.oneLine(), mask: e.Mask()}
}

func (e *Editor) lineView() editorLineView {
	return editorLineView{
		editorProjectionKey: e.projectionKey(),
		value:               e.Text(), placeholder: e.placeholder,
		cursorByte: e.byteOffset, anchorByte: e.anchor.Byte, selecting: e.selection != editorSelectionNone,
		focused: e.element.Focused(), left: e.geometry.read(&e.presentation).left,
		gutter: e.gutter, cursorStyle: e.cursorStyle,
	}
}

func (e *Editor) shown() string {
	e.ensure()
	return e.lineView().shown()
}

// sourceByteOf maps a byte offset in the shown one-line text to the source byte
// it stands for. A mask shows one copy of itself for every source cluster.
func (e *Editor) sourceByteOf(shownByte int) int {
	if e.Mask() == "" {
		return shownByte
	}
	line := e.document().lines[0]
	if shownByte <= 0 {
		return 0
	}
	want := shownByte / len(e.Mask())
	seen := 0
	for offset := range text.Clusters(line) {
		if seen == want {
			return offset
		}
		seen++
	}
	return len(line)
}

func (e *Editor) caretAtColumn(x int) Caret {
	byteOffset := e.sourceByteOf(text.OffsetAt(e.shown(), layout.Translate(x, e.geometry.read(&e.presentation).left)))
	return Caret{Byte: e.document().snapElement(0, byteOffset, true)}
}
