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
		cursor: Caret{Line: e.line, Col: e.col}, anchor: e.anchor, rowEnd: e.rowEnd,
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
