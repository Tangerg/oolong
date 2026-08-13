package headless

// editorHistory owns the two bounded stacks that make an edit reversible.
//
// Keeping stack mutation here matters for more than tidiness. An editor state owns
// slices of text and marks, so a popped value must be cleared from spare capacity and
// an evicted value must not remain before a subslice. Those are properties of a
// history, not details every editing operation should remember.
type editorHistory struct {
	undo, redo []editorState
}

func (h *editorHistory) clear() {
	clear(h.undo)
	clear(h.redo)
	h.undo = nil
	h.redo = nil
}

func (h *editorHistory) canBack() bool    { return len(h.undo) > 0 }
func (h *editorHistory) canForward() bool { return len(h.redo) > 0 }

func (h *editorHistory) record(state editorState) {
	h.undo = pushEditorState(h.undo, state)
	clear(h.redo)
	h.redo = nil
}

func (h *editorHistory) back(current editorState) (editorState, bool) {
	previous, ok := popEditorState(&h.undo)
	if !ok {
		return editorState{}, false
	}
	h.redo = pushEditorState(h.redo, current)
	return previous, true
}

func (h *editorHistory) forward(current editorState) (editorState, bool) {
	next, ok := popEditorState(&h.redo)
	if !ok {
		return editorState{}, false
	}
	h.undo = pushEditorState(h.undo, current)
	return next, true
}

func pushEditorState(stack []editorState, state editorState) []editorState {
	stack = append(stack, state)
	if len(stack) <= maxUndo {
		return stack
	}
	drop := len(stack) - maxUndo
	copy(stack, stack[drop:])
	clear(stack[len(stack)-drop:])
	return stack[:maxUndo]
}

func popEditorState(stack *[]editorState) (editorState, bool) {
	if len(*stack) == 0 {
		return editorState{}, false
	}
	last := len(*stack) - 1
	state := (*stack)[last]
	(*stack)[last] = editorState{}
	*stack = (*stack)[:last]
	return state, true
}
