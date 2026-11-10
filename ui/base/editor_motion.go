package base

import (
	"image"
	"unicode"
	"unicode/utf8"

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
	if e.byteOffset > 0 {
		e.byteOffset = e.document().snapElementBoundary(e.line, text.PrevCluster(e.document().lines[e.line], e.byteOffset), false)
		return
	}
	if e.line > 0 {
		e.line--
		e.byteOffset = len(e.document().lines[e.line])
	}
}

func (e *Editor) MoveRight() {
	e.beginViewUpdate()
	defer e.endViewUpdate()
	defer e.revealCursor()
	e.ensure()
	e.endTyping()
	e.desiredColumn = editorColumn{}
	if e.byteOffset < len(e.document().lines[e.line]) {
		e.byteOffset = e.document().snapElementBoundary(e.line, nextClusterBoundary(e.document().lines[e.line], e.byteOffset), true)
		return
	}
	if e.line < len(e.document().lines)-1 {
		e.line++
		e.byteOffset = 0
	}
}

func (e *Editor) MoveWordLeft() {
	e.beginViewUpdate()
	defer e.endViewUpdate()
	defer e.revealCursor()
	e.ensure()
	e.endTyping()
	e.desiredColumn = editorColumn{}
	if e.byteOffset == 0 {
		e.MoveLeft()
		return
	}
	e.byteOffset = e.document().snapElementBoundary(e.line, wordStart(e.document().lines[e.line], e.byteOffset), false)
}

func (e *Editor) MoveWordRight() {
	e.beginViewUpdate()
	defer e.endViewUpdate()
	defer e.revealCursor()
	e.ensure()
	e.endTyping()
	e.desiredColumn = editorColumn{}
	if e.byteOffset == len(e.document().lines[e.line]) {
		e.MoveRight()
		return
	}
	e.byteOffset = e.document().snapElementBoundary(e.line, wordEnd(e.document().lines[e.line], e.byteOffset), true)
}

func (e *Editor) MoveLineStart() {
	e.beginViewUpdate()
	defer e.endViewUpdate()
	defer e.revealCursor()
	e.ensure()
	e.endTyping()
	e.desiredColumn = editorColumn{}
	e.byteOffset = 0
}

func (e *Editor) MoveLineEnd() {
	e.beginViewUpdate()
	defer e.endViewUpdate()
	defer e.revealCursor()
	e.ensure()
	e.endTyping()
	e.desiredColumn = editorColumn{}
	e.byteOffset = len(e.document().lines[e.line])
}

func wordStart(s string, i int) int {
	start := 0
	inWord := false
	for at, cluster := range text.Clusters(s[:i]) {
		word := isWord(cluster)
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
		word := isWord(cluster)
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

func isWord(cluster string) bool {
	r, _ := utf8.DecodeRuneInString(cluster)
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
}

// A soft break has two visual positions at one byte offset; pointer affinity chooses the earlier row.
func (e *Editor) rowAt(width int) (row, column int) {
	e.rows(width)
	affinity := text.Forward
	if e.prefersRowEnd() {
		affinity = text.Backward
	}
	point := e.layout.projection.Caret(text.Position{Line: e.line, Byte: e.byteOffset, Affinity: affinity})
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
		column := text.ColumnOf(e.document().lines[e.line], e.byteOffset)
		column = e.desiredColumn.resolve(column)
		target := min(max(e.line+delta, 0), len(e.document().lines)-1)
		if target == e.line {
			return
		}
		e.line = target
		e.byteOffset = e.document().snapElementBoundary(target, text.OffsetAt(e.document().lines[target], column), forward)
		e.desiredColumn = editorColumn{cells: column, valid: true}
		return
	}
	row, column := e.rowAt(width)
	column = e.desiredColumn.resolve(column)
	target := row + delta
	if target < 0 || target >= len(e.rows(width)) {
		return
	}
	e.line, e.byteOffset = e.offsetIn(width, target, column)
	e.byteOffset = e.document().snapElementBoundary(e.line, e.byteOffset, forward)
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
