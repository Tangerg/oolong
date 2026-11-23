package base

import (
	"image"

	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/keymap"
	"github.com/Tangerg/oolong/ui/core/text"
)

type editorColumn struct {
	cells int
	valid bool
}

func (c editorColumn) resolve(current int) int {
	if c.valid {
		return c.cells
	}
	return current
}

func (e *Editor) MoveLeft() {
	e.beginViewUpdate()
	defer e.endViewUpdate()
	defer e.revealCursor()
	e.ensure()
	e.endTyping()
	e.desiredColumn = editorColumn{}
	if e.caret.Byte > 0 {
		e.caret.Byte = e.document().snapElementBoundary(e.caret.Line, text.PrevCluster(e.document().lines[e.caret.Line], e.caret.Byte), false)
		return
	}
	if e.caret.Line > 0 {
		e.caret.Line--
		e.caret.Byte = len(e.document().lines[e.caret.Line])
	}
}

func (e *Editor) MoveRight() {
	e.beginViewUpdate()
	defer e.endViewUpdate()
	defer e.revealCursor()
	e.ensure()
	e.endTyping()
	e.desiredColumn = editorColumn{}
	if e.caret.Byte < len(e.document().lines[e.caret.Line]) {
		e.caret.Byte = e.document().snapElementBoundary(e.caret.Line, nextClusterBoundary(e.document().lines[e.caret.Line], e.caret.Byte), true)
		return
	}
	if e.caret.Line < len(e.document().lines)-1 {
		e.caret.Line++
		e.caret.Byte = 0
	}
}

func (e *Editor) MoveWordLeft() {
	e.beginViewUpdate()
	defer e.endViewUpdate()
	defer e.revealCursor()
	e.ensure()
	e.endTyping()
	e.desiredColumn = editorColumn{}
	if e.caret.Byte == 0 {
		e.MoveLeft()
		return
	}
	e.caret.Byte = e.document().snapElementBoundary(e.caret.Line, wordStart(e.document().lines[e.caret.Line], e.caret.Byte), false)
}

func (e *Editor) MoveWordRight() {
	e.beginViewUpdate()
	defer e.endViewUpdate()
	defer e.revealCursor()
	e.ensure()
	e.endTyping()
	e.desiredColumn = editorColumn{}
	if e.caret.Byte == len(e.document().lines[e.caret.Line]) {
		e.MoveRight()
		return
	}
	e.caret.Byte = e.document().snapElementBoundary(e.caret.Line, wordEnd(e.document().lines[e.caret.Line], e.caret.Byte), true)
}

func (e *Editor) MoveLineStart() {
	e.beginViewUpdate()
	defer e.endViewUpdate()
	defer e.revealCursor()
	e.ensure()
	e.endTyping()
	e.desiredColumn = editorColumn{}
	e.caret.Byte = 0
}

func (e *Editor) MoveLineEnd() {
	e.beginViewUpdate()
	defer e.endViewUpdate()
	defer e.revealCursor()
	e.ensure()
	e.endTyping()
	e.desiredColumn = editorColumn{}
	e.caret.Byte = len(e.document().lines[e.caret.Line])
}

func wordStart(s string, i int) int {
	start := 0
	inWord := false
	for at, cluster := range text.Clusters(s[:i]) {
		word := text.ClassOf(cluster).InWord()
		if word && !inWord {
			start = at
		}
		inWord = word
	}
	return start
}

func wordEnd(s string, i int) int {
	found := false
	for at, cluster := range text.Clusters(s[i:]) {
		word := text.ClassOf(cluster).InWord()
		if found && !word {
			return i + at
		}
		found = found || word
	}
	return len(s)
}

// Known grapheme boundaries allow segmentation to resume here without rescanning the prefix.
func nextClusterBoundary(s string, at int) int {
	if at >= len(s) {
		return len(s)
	}
	for _, cluster := range text.Clusters(s[at:]) {
		return at + len(cluster)
	}
	return at
}

// A soft break has two visual positions at one byte offset; pointer affinity chooses the earlier row.
func (e *Editor) rowAt(width int) (row, column int) {
	e.rows(width)
	affinity := text.Forward
	if e.prefersRowEnd() {
		affinity = text.Backward
	}
	point := e.layout.projection.Caret(text.Position{Line: e.caret.Line, Byte: e.caret.Byte, Affinity: affinity})
	return point.Y, point.X
}

func (e *Editor) offsetIn(width, row, column int) (line, byteOffset int) {
	e.rows(width)
	position := e.layout.projection.Hit(image.Pt(column, row))
	return position.Line, position.Byte
}

func (e *Editor) MoveUp() { e.moveRow(-1) }

func (e *Editor) MoveDown() { e.moveRow(1) }

// Preserve the desired column across short rows so repeated vertical movement cannot drift left.
func (e *Editor) moveRow(delta int) {
	e.beginViewUpdate()
	defer e.endViewUpdate()
	defer e.revealCursor()
	e.ensure()
	e.endTyping()
	width := e.geometry.read(&e.presentation).width
	// Snapping toward the movement avoids returning to the same wrapped atomic row.
	forward := delta > 0
	if width <= 0 {
		// Nothing has been drawn yet, so there are no visual rows to move through.
		// Logical lines are the best available answer, but the horizontal coordinate
		// is still a terminal column rather than a byte offset. Copying the byte offset
		// to a UTF-8 line can put the cursor in the middle of a rune.
		column := text.ColumnOf(e.document().lines[e.caret.Line], e.caret.Byte)
		column = e.desiredColumn.resolve(column)
		target := min(max(e.caret.Line+delta, 0), len(e.document().lines)-1)
		if target == e.caret.Line {
			return
		}
		e.caret.Line = target
		e.caret.Byte = e.document().snapElementBoundary(target, text.OffsetAt(e.document().lines[target], column), forward)
		e.desiredColumn = editorColumn{cells: column, valid: true}
		return
	}
	row, column := e.rowAt(width)
	column = e.desiredColumn.resolve(column)
	target := row + delta
	if target < 0 || target >= len(e.rows(width)) {
		return
	}
	e.caret.Line, e.caret.Byte = e.offsetIn(width, target, column)
	e.caret.Byte = e.document().snapElementBoundary(e.caret.Line, e.caret.Byte, forward)
	e.desiredColumn = editorColumn{cells: column, valid: true}
}

// editorMovement is one caret movement: the action that moves, the action that
// extends the selection by the same movement, and the chord both answer to by
// default, with Shift added for extension. Do and DefaultEditorKeys both read this
// one table, so a movement cannot be bound without being runnable or the reverse.
type editorMovement struct {
	move, extend keymap.Action
	chord        input.Chord
	run          func(*Editor)
}

var editorMovements = [...]editorMovement{
	{MoveLeft, ExtendLeft, input.Chord{Code: input.Left}, (*Editor).MoveLeft},
	{MoveRight, ExtendRight, input.Chord{Code: input.Right}, (*Editor).MoveRight},
	{MoveUp, ExtendUp, input.Chord{Code: input.Up}, (*Editor).MoveUp},
	{MoveDown, ExtendDown, input.Chord{Code: input.Down}, (*Editor).MoveDown},
	{MoveWordLeft, ExtendWordLeft, input.Alt.Rune('b'), (*Editor).MoveWordLeft},
	{MoveWordRight, ExtendWordRight, input.Alt.Rune('f'), (*Editor).MoveWordRight},
	{MoveLineStart, ExtendLineStart, input.Ctrl.Rune('a'), (*Editor).MoveLineStart},
	{MoveLineEnd, ExtendLineEnd, input.Ctrl.Rune('e'), (*Editor).MoveLineEnd},
}
