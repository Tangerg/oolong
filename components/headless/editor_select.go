package headless

import (
	"slices"
	"strings"

	"github.com/Tangerg/oolong/core/keymap"
)

// Caret is a position in an editor's text: a logical line, and a byte offset into it.
//
// It is not a [Point]. A transcript numbers visual rows, because everything it
// answers is about what is on the screen; an editor's text has lines of its own that
// wrapping turns into rows, and a position in the text has to survive the window
// changing width. The two coordinate spaces are different questions, and one type for
// both would let an answer to one be passed as an answer to the other.
type Caret struct{ Line, Col int }

// Before reports whether c comes earlier in the text than d.
func (c Caret) Before(d Caret) bool {
	return c.Line < d.Line || (c.Line == d.Line && c.Col < d.Col)
}

// Clipboard is where an editor's copy and cut go.
//
// A runtime adapter commonly provides it, but the interface is declared here where
// it is consumed. A caller with somewhere else to put text is equally valid.
type Clipboard interface {
	// Copy puts text where a paste would find it, reporting false for text it will
	// not carry.
	Copy(text string) bool
	// Paste asks for what is there. The answer arrives later, as an [input.Paste]
	// among the editor's ordinary events, which is why nothing is returned.
	Paste()
}

// Anchor begins or continues a selection at the cursor.
//
// A selection is not a separate mode with commands of its own. It is what movement
// means while the shift key is held, so every way of moving a cursor selects with
// shift and none of them had to be taught to — see [Editor.Handle].
func (e *Editor) Anchor() {
	e.breakContinuation()
	if !e.selecting {
		e.anchor, e.selecting = Caret{Line: e.line, Col: e.col}, true
	}
}

// SelectNone drops the selection, leaving the cursor where it is.
func (e *Editor) SelectNone() {
	e.breakContinuation()
	e.selecting = false
}

// SelectAll selects the whole text.
func (e *Editor) SelectAll() {
	e.ensure()
	e.endTyping()
	e.anchor, e.selecting = Caret{}, true
	e.line = len(e.lines) - 1
	e.col = len(e.lines[e.line])
	e.wantColumn = -1
}

// Selection is the selected range in reading order, and whether there is one.
//
// It reports false for a selection of nothing, which is what a shift-arrow pressed
// and then taken back leaves: an anchor at the cursor is not a selection, and
// treating it as one would make a copy put an empty string on the clipboard.
func (e *Editor) Selection() (start, end Caret, ok bool) {
	if !e.selecting {
		return Caret{}, Caret{}, false
	}
	cursor := Caret{Line: e.line, Col: e.col}
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
	e.ensure()
	if start.Line == end.Line {
		return e.lines[start.Line][start.Col:end.Col]
	}
	var b strings.Builder
	b.WriteString(e.lines[start.Line][start.Col:])
	for i := start.Line + 1; i < end.Line; i++ {
		b.WriteByte('\n')
		b.WriteString(e.lines[i])
	}
	b.WriteByte('\n')
	b.WriteString(e.lines[end.Line][:end.Col])
	return b.String()
}

// DeleteSelection removes the selected text and reports whether there was any.
func (e *Editor) DeleteSelection() bool {
	if _, _, ok := e.Selection(); !ok {
		return false
	}
	e.snapshot()
	e.endTyping()
	return e.dropSelection()
}

// replaceRange puts s where the range was, leaving the cursor at the end of what it
// put there. The caller has taken the snapshot.
func (e *Editor) replaceRange(start, end Caret, s string) {
	// The cut is described against the document it is a cut of, so the marks move
	// before the lines do. What goes in its place is a second edit, made by splice
	// below, against the document the cut left.
	e.removed(start, end, "")
	head := e.lines[start.Line][:start.Col]
	tail := e.lines[end.Line][end.Col:]
	e.lines = slices.Replace(e.lines, start.Line, end.Line+1, head+tail)
	e.line, e.col = start.Line, start.Col
	e.wantColumn = -1
	e.invalidate()
	if s != "" {
		e.splice(s)
	}
}

// Copy puts the selection where a paste would find it, and reports whether anything
// was sent. Nothing selected sends nothing, which is not a failure.
func (e *Editor) Copy() bool {
	if e.Clipboard == nil {
		return false
	}
	selected := e.Selected()
	if selected == "" {
		return false
	}
	return e.Clipboard.Copy(selected)
}

// Cut copies the selection and removes it.
//
// The text is removed only if the clipboard took it. A cut that emptied the field
// into a clipboard that refused it would lose the text with nothing to paste back,
// and a terminal is free to refuse.
func (e *Editor) Cut() bool {
	if !e.Copy() {
		return false
	}
	return e.DeleteSelection()
}

// Paste asks the clipboard for its contents. What comes back arrives later as an
// ordinary paste event, which this editor already inserts.
func (e *Editor) Paste() {
	if e.Clipboard != nil {
		e.Clipboard.Paste()
	}
}

// move runs an action if it is a way of moving, reporting whether it was one.
//
// It is separate from the rest of [Editor.Do] because it is asked twice: once for what
// the keystroke named, and once for what it named with the shift taken off it, which is
// what makes every movement a way of selecting. Selecting is this without the selection
// being let go of first.
func (e *Editor) move(action keymap.Action) bool {
	switch action {
	case MoveLeft:
		e.MoveLeft()
	case MoveRight:
		e.MoveRight()
	case MoveUp:
		e.MoveUp()
	case MoveDown:
		e.MoveDown()
	case MoveWordLeft:
		e.MoveWordLeft()
	case MoveWordRight:
		e.MoveWordRight()
	case MoveLineStart:
		e.MoveLineStart()
	case MoveLineEnd:
		e.MoveLineEnd()
	default:
		return false
	}
	return true
}

// dropSelection removes the selected text without taking a snapshot, so that an edit
// which replaces a selection is one step to undo rather than two.
func (e *Editor) dropSelection() bool {
	start, end, ok := e.Selection()
	if !ok {
		e.selecting = false
		return false
	}
	e.selecting = false
	e.replaceRange(start, end, "")
	return true
}
