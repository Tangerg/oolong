package base

import "github.com/Tangerg/oolong/ui/core/text"

// Insert inserts text at the cursor as one undo step, splitting lines at newlines.
func (e *Editor) Insert(s string) { e.insert(s, false) }

func (e *Editor) insert(value string, typed bool) {
	e.beginViewUpdate()
	defer e.endViewUpdate()
	if value == "" {
		// A handled no-op ends the interaction however the key was encoded.
		e.endTyping()
		return
	}
	e.ensure()
	start, end := e.replacementRange()
	history := recordEdit
	if typed {
		history = coalesceEdit
	}
	e.document().replace(e, editReplacement{start: start, end: end, text: value, history: history}, e.finishReplacement)
}

// Replace swaps the byte range [start, end) of line for value, and leaves the
// cursor after what was put in. The line is clamped to the document, and the range
// to the line, expanded to whole grapheme clusters; a terminal cursor cannot
// address half of one. The line is explicit, as a [Token] carries it: a range read
// from one line must never edit whichever line the cursor has since moved to.
//
// It is one edit rather than a delete and an insert so that it is one step to undo:
// accepting a completion is one thing the user did, and taking it back should not
// take two. A token never spans lines, which is why the range does not either.
func (e *Editor) Replace(line, start, end int, value string) {
	e.beginViewUpdate()
	defer e.endViewUpdate()
	e.ensure()
	line = min(max(line, 0), len(e.document().lines)-1)
	e.document().replace(e, editReplacement{
		start: Caret{Line: line, Byte: start}, end: Caret{Line: line, Byte: end}, text: value, expandClusters: true,
	}, e.finishReplacement)
}

func (e *Editor) InsertRune(r rune) {
	e.beginViewUpdate()
	defer e.endViewUpdate()
	// InsertRune cannot create logical lines; Printable preserves tabs but not line breaks.
	e.insert(text.Printable(string(r)), true)
}

// Newline splits the line at the cursor, and does nothing at all in a field that holds
// one line.
func (e *Editor) Newline() {
	e.beginViewUpdate()
	defer e.endViewUpdate()
	if e.oneLine() {
		e.endTyping()
		return
	}
	e.Insert("\n")
}

// DeleteBack removes the cluster before the cursor, or joins this line to the one
// above when the cursor is at the start of a line.
func (e *Editor) DeleteBack() {
	e.beginViewUpdate()
	defer e.endViewUpdate()
	e.ensure()
	if e.byteOffset > 0 {
		at := text.PrevCluster(e.document().lines[e.line], e.byteOffset)
		if element, inside := e.document().insideElement(e.line, at); inside {
			at = element.Start
		}
		e.document().replace(e, editReplacement{
			start: Caret{Line: e.line, Byte: at}, end: Caret{Line: e.line, Byte: e.byteOffset}, history: coalesceEdit,
		}, e.finishReplacement)
		return
	}
	if e.line == 0 {
		e.endTyping()
		return
	}
	above := e.document().lines[e.line-1]
	e.document().replace(e, editReplacement{
		start: Caret{Line: e.line - 1, Byte: len(above)}, end: Caret{Line: e.line},
	}, e.finishReplacement)
}

// DeleteForward removes the cluster after the cursor, or joins the line below.
func (e *Editor) DeleteForward() {
	e.beginViewUpdate()
	defer e.endViewUpdate()
	e.ensure()
	current := e.document().lines[e.line]
	if e.byteOffset < len(current) {
		at := nextClusterBoundary(current, e.byteOffset)
		if element, inside := e.ElementAt(e.line, e.byteOffset); inside {
			at = element.End
		}
		e.document().replace(e, editReplacement{
			start: Caret{Line: e.line, Byte: e.byteOffset}, end: Caret{Line: e.line, Byte: at},
		}, e.finishReplacement)
		return
	}
	if e.line == len(e.document().lines)-1 {
		e.endTyping()
		return
	}
	e.document().replace(e, editReplacement{
		start: Caret{Line: e.line, Byte: len(current)}, end: Caret{Line: e.line + 1},
	}, e.finishReplacement)
}

func (e *Editor) DeleteWordBack() {
	e.beginViewUpdate()
	defer e.endViewUpdate()
	e.ensure()
	end := Caret{Line: e.line, Byte: e.byteOffset}
	start := end
	if start.Byte > 0 {
		at := wordStart(e.document().lines[e.line], e.byteOffset)
		start.Byte = e.document().snapElementBoundary(e.line, at, false)
	} else if start.Line > 0 {
		start.Line--
		start.Byte = len(e.document().lines[start.Line])
	}
	e.killRange(start, end, true)
}

func (e *Editor) Undo() {
	e.beginViewUpdate()
	defer e.endViewUpdate()
	e.document().undo(e)
}

func (e *Editor) Redo() {
	e.beginViewUpdate()
	defer e.endViewUpdate()
	e.document().redo(e)
}

// InsertElement inserts an atomic text unit and returns it. Line breaks become spaces
// so its line-local range remains valid. An ordinary separator follows; a leading
// separator prevents joining the preceding grapheme. Empty bodies insert nothing.
// Identities are never reused; exhaustion panics before insertion.
func (e *Editor) InsertElement(kind ElementKind, body string) Element {
	e.beginViewUpdate()
	defer e.endViewUpdate()
	e.ensure()
	start, end := e.replacementRange()
	return e.document().insertElement(e, start, end, kind, body, e.finishReplacement)
}

// Elements is every element in the text, in the order they appear. The slice is a
// copy: a caller cannot move an element by writing to it.
func (e *Editor) Elements() []Element {
	out := make([]Element, 0, len(e.document().marks))
	for _, m := range e.document().marks {
		out = append(out, e.document().elementOf(m))
	}
	return out
}

// ElementAt is the element covering a position, and whether there is one. The end is
// exclusive, so the position just after an element is outside it. Out-of-range
// positions return false.
func (e *Editor) ElementAt(line, byteOffset int) (Element, bool) {
	lines := e.document().lines
	if line < 0 || line >= len(lines) || byteOffset < 0 || byteOffset >= len(lines[line]) {
		return Element{}, false
	}
	at := offsetInLines(lines, Caret{Line: line, Byte: byteOffset})
	for _, m := range e.document().marks {
		if m.Covers(at) {
			return e.document().elementOf(m), true
		}
	}
	return Element{}, false
}

// RemoveElement deletes an element's text and forgets it, reporting whether it was
// there to remove.
func (e *Editor) RemoveElement(id ElementID) bool {
	e.beginViewUpdate()
	defer e.endViewUpdate()
	start, end, found := e.document().elementRemoval(id)
	if !found {
		return false
	}
	e.document().replace(e, editReplacement{start: start, end: end}, e.finishReplacement)
	return true
}

// RetainedElementIDs returns identities reachable from the document or undo/redo
// history. Applications may release associated payloads only after they disappear
// from this set. The returned slice is owned by the caller.
func (e *Editor) RetainedElementIDs() []ElementID {
	return e.document().retainedElementIDs()
}

// ForgetHistory ends the lifetime of edits no longer available to Undo or Redo.
// Current document elements remain live.
func (e *Editor) ForgetHistory() {
	e.beginViewUpdate()
	defer e.endViewUpdate()
	e.endTyping()
	e.document().forgetHistory()
}
