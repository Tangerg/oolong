package base

// Nested commands publish only after cursor, selection and scroll agree. Document
// commits reset this baseline after mapping every shared view, before observers run.
type editorViewUpdates struct {
	depth  int
	before editorViewState
}

type editorViewState struct {
	cursor, anchor Caret
	selection      editorSelection
	rowEnd         bool
	scrollRevision uint64
}

func (e *Editor) viewState() editorViewState {
	return editorViewState{
		cursor: Caret{Line: e.line, Byte: e.byteOffset}, anchor: e.anchor,
		selection: e.selection, rowEnd: e.prefersRowEnd(),
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
// prepareEdit settles interaction state before an edit. Any edit ends a pointer
// drag: the edit replaces the selection the drag was extending, and a drag left
// running would move the caret with no selection to extend. Only continuous typing
// keeps the caret's row-end affinity.
func (e *Editor) prepareEdit(continuous bool) {
	e.endDrag()
	if !continuous {
		e.rowEnd = nil
	}
	e.breakContinuation()
}

func (e *Editor) finishReplacement(at Caret, changed bool) {
	if changed {
		e.layout.stale = true
	}
	e.selection = editorSelectionNone
	e.line = min(max(at.Line, 0), len(e.document().lines)-1)
	e.byteOffset = e.document().snapElement(e.line, at.Byte, true)
	e.desiredColumn = editorColumn{}
	e.revealCursor()
}

func (e *Editor) restoreCaret(at Caret) {
	e.line, e.byteOffset = at.Line, at.Byte
	e.selection = editorSelectionNone
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
	// A peer's edit moves this view's caret but not its reader. Only a reveal this
	// view still owes its own cursor follows the caret to its new place; a reader
	// who scrolled away stays where they are.
	if initiator || e.cursorRevealPending() {
		e.revealCursor()
	}
}

func (e *Editor) policyChanged() {
	e.endTyping()
	e.layout.stale = true
	e.revealCursor()
}
