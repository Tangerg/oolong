package base

import (
	"image"

	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/text"
)

type editorPresentation struct {
	content                    editorProjectionKey
	width, gutter, first, left int
	visible                    image.Rectangle
}

func (p editorPresentation) textPoint(point image.Point) (image.Point, bool) {
	if p.width <= 0 || point.X < p.gutter {
		return image.Point{}, false
	}
	point.X = layout.Relative(point.X, p.gutter)
	return point, true
}

// At maps field-local coordinates to a caret, accounting for scrolling and gutter.
// It shares the cursor and selection wrap so hit testing agrees with rendering.
func (e *Editor) At(x, y, width int) (Caret, bool) {
	gutter := e.gutterWidth()
	if x < gutter {
		return Caret{}, false
	}
	return e.at(x-gutter, y, layout.Remaining(width, gutter))
}

func (e *Editor) at(x, y, width int) (Caret, bool) {
	if width <= 0 || x < 0 || y < 0 {
		return Caret{}, false
	}
	e.ensure()
	if e.oneLine() {
		return e.caretAtColumn(x), true
	}
	e.rows(width)
	presented := e.geometry.read(&e.presentation)
	first := 0
	if presented.width == width {
		first = presented.first
	}
	position := e.layout.projection.Hit(image.Pt(x, layout.Sum(first, y)))
	return Caret{Line: position.Line, Byte: e.document().snapElement(position.Line, position.Byte, true)}, true
}

func (e *Editor) handleMouse(ev input.Mouse, presented editorPresentation) bool {
	e.beginViewUpdate()
	defer e.endViewUpdate()
	switch ev.Action {
	case input.MouseCancel:
		e.endDrag()
		return false
	case input.MouseUp:
		if !ev.Releases(input.ButtonLeft) {
			return false
		}
		return e.endDrag()
	case input.MouseDown:
		e.endDrag()
		if ev.Button != input.ButtonLeft || !ev.Pos.In(presented.visible) {
			return false
		}
	case input.MouseDrag:
		if e.selection != editorDragging || !ev.Drags(input.ButtonLeft) {
			return false
		}
	default:
		return false
	}
	if presented.content != e.projectionKey() {
		return false
	}
	point, ok := presented.textPoint(ev.Pos)
	if !ok {
		return false
	}
	at, ok := e.at(point.X, point.Y, presented.width)
	if !ok {
		return false
	}
	if ev.Action == input.MouseDown {
		e.endTyping()
		e.anchor, e.selection = at, editorDragging
		e.rowEnd = nil
		if e.pastRowEnd(point.X, point.Y, presented.width) {
			e.rowEnd = &at
		}
	}
	e.line, e.byteOffset = at.Line, at.Byte
	e.desiredColumn = editorColumn{}
	e.revealCursor()
	return true
}

func (e *Editor) pastRowEnd(x, y, width int) bool {
	rows := e.rows(width)
	presented := e.geometry.read(&e.presentation)
	first := 0
	if presented.width == width {
		first = presented.first
	}
	index := layout.Sum(first, y)
	if index < 0 || index >= len(rows) || e.layout.lastOfLine(index) {
		return false
	}
	r := rows[index]
	return x >= text.Width(e.document().lines[r.line][r.start:r.end])
}

func (e *Editor) prefersRowEnd() bool {
	return e.rowEnd != nil && *e.rowEnd == Caret{Line: e.line, Byte: e.byteOffset}
}
