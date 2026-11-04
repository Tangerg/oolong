package base

// Nested commands publish only after cursor, selection and scroll agree. Document
// commits reset this baseline after mapping every shared view, before observers run.
type editorViewUpdates struct {
	depth  int
	before editorViewState
}

type editorViewState struct {
	cursor, anchor, rowEnd        Caret
	selecting, dragging, affinity bool
	scrollRevision                uint64
}

func (e *Editor) viewState() editorViewState {
	return editorViewState{
		cursor: Caret{Line: e.line, Byte: e.byteOffset}, anchor: e.anchor, rowEnd: e.rowEnd,
		selecting: e.selecting, dragging: e.dragging, affinity: e.rowEndSet,
		scrollRevision: e.scroll.revision,
	}
}

func (e *Editor) beginViewUpdate() {
	e.requireMutable()
	if e.updates.depth == 0 {
		e.updates.before = e.viewState()
	}
	e.updates.depth++
}

func (e *Editor) endViewUpdate() {
	e.updates.depth--
	if e.updates.depth == 0 && e.updates.before != e.viewState() {
		e.publishViewUpdate()
	}
}

func (e *Editor) publishViewUpdate() {
	e.updates.before = e.viewState()
	e.element.Invalidate()
}

func (e *Editor) requireMutable() {
	requireMutable(e.element)
	e.document().requireMutable()
}

func (e *Editor) replacementRange() (Caret, Caret) {
	if start, end, selected := e.Selection(); selected {
		return start, end
	}
	caret := Caret{Line: e.line, Byte: e.byteOffset}
	return caret, caret
}

// The session calls this only after every refusal boundary, so a rejected edit
// cannot end a typing group or lose a view's drag and yank continuation.
func (e *Editor) prepareEdit(continuous bool) {
	if !continuous {
		e.dragging = false
		e.rowEndSet = false
	}
	e.breakContinuation()
}

func (e *Editor) finishReplacement(at Caret, changed bool) {
	if changed {
		e.layout.stale = true
	}
	e.selecting = false
	e.line = min(max(at.Line, 0), len(e.document().lines)-1)
	e.byteOffset = e.document().snapElement(e.line, at.Byte, true)
	e.desiredColumn = editorColumn{}
	e.revealCursor()
}

func (e *Editor) restoreCaret(at Caret) {
	e.line, e.byteOffset = at.Line, at.Byte
	e.selecting = false
}

func (e *Editor) documentChanged(before []string, change ChangeMap, initiator bool) {
	if !initiator {
		cursor := change.Position(offsetInLines(before, Caret{Line: e.line, Byte: e.byteOffset}))
		anchor := change.Position(offsetInLines(before, e.anchor))
		position := e.document().caretAt(cursor)
		e.line, e.byteOffset = position.Line, e.document().snapElement(position.Line, position.Byte, true)
		e.anchor = e.document().caretAt(anchor)
		e.anchor.Byte = e.document().snapElement(e.anchor.Line, e.anchor.Byte, false)
		e.endTyping()
	}
	e.layout.stale = true
	e.desiredColumn = editorColumn{}
	e.revealCursor()
}

func (e *Editor) policyChanged() {
	e.endTyping()
	e.layout.stale = true
	e.revealCursor()
}
