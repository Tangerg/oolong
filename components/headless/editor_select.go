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
	// Paste asks for what is there and reports whether the request was accepted. The
	// answer arrives later, as an [input.Paste] among the editor's ordinary events.
	Paste() bool
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
	return e.textBetween(start, end)
}

// textBetween is the text in a range in reading order.
func (e *Editor) textBetween(start, end Caret) string {
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
	start, end, ok := e.Selection()
	if !ok {
		return false
	}
	e.snapshot()
	e.endTyping()
	e.replaceRange(start, end, "")
	return true
}

// prepareReplacement applies the editor's one-line rule and reports whether replacing
// a range changes text or atomic elements. Its returned text is the canonical value
// replaceRange consumes, so normalization occurs at this one boundary.
//
// Comparing text alone is not enough: replacing a chip with the same visible word
// removes the element identity and is therefore a semantic change too. Conversely,
// an empty edit inside a chip is still empty and must not destroy it.
func (e *Editor) prepareReplacement(start, end Caret, s string) (string, bool) {
	s = e.flatten(s)
	if e.textBetween(start, end) != s {
		return s, true
	}
	if start == end && s == "" {
		return s, false
	}
	from, to := e.offsetOf(start), e.offsetOf(end)
	for _, mark := range e.marks {
		if from < mark.End && to > mark.Start {
			return s, true
		}
	}
	return s, false
}

// finishReplacement settles the interaction state common to a replacement whether
// or not its bytes changed. The cursor belongs after what was put there and the old
// selection no longer describes an active range. Revision, history and layout remain
// separate because an identity replacement changes none of them.
func (e *Editor) finishReplacement(at Caret) {
	e.selecting = false
	e.line, e.col = at.Line, at.Col
	e.wantColumn = -1
}

// replaceRange is the one operation that changes editor text. Insertion is an empty
// range, deletion has empty replacement text, and replacement is both. The caller
// has established that the range changes semantic content and taken any undo snapshot
// it needs.
func (e *Editor) replaceRange(start, end Caret, s string) {
	e.requireContentRevision()
	e.removed(start, end, s)
	head := e.lines[start.Line][:start.Col]
	tail := e.lines[end.Line][end.Col:]
	if !strings.Contains(s, "\n") {
		e.lines = slices.Replace(e.lines, start.Line, end.Line+1, ownedEditorLine(head, s, tail))
		e.finishReplacement(Caret{Line: start.Line, Col: start.Col + len(s)})
		e.contentChanged()
		return
	}
	parts := strings.Split(s, "\n")
	inserted := make([]string, len(parts))
	inserted[0] = ownedEditorLine(head, parts[0])
	for i := 1; i < len(parts); i++ {
		inserted[i] = strings.Clone(parts[i])
	}
	last := len(inserted) - 1
	cursor := Caret{Line: start.Line + last, Col: len(inserted[last])}
	inserted[last] = ownedEditorLine(inserted[last], tail)
	e.lines = slices.Replace(e.lines, start.Line, end.Line+1, inserted...)
	e.finishReplacement(cursor)
	e.contentChanged()
}

// ownedEditorLine joins pieces into storage owned by the surviving document. A range
// edit commonly keeps only the prefix or suffix of a large source line; retaining the
// source allocation would make deleting text keep the deleted bytes alive.
func ownedEditorLine(parts ...string) string {
	length := 0
	for _, part := range parts {
		length += len(part)
	}
	var line strings.Builder
	line.Grow(length)
	for _, part := range parts {
		line.WriteString(part)
	}
	return line.String()
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

// Paste asks the clipboard for its contents and reports whether the request was
// accepted. What comes back arrives later as an ordinary paste event, which this
// editor already inserts.
func (e *Editor) Paste() bool {
	return e.Clipboard != nil && e.Clipboard.Paste()
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
