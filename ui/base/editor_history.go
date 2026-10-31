package base

import "weak"

// editorHistory owns the two bounded stacks that make an edit reversible.
//
// Keeping stack mutation here matters for more than tidiness. An editor state owns
// slices of text and marks, so a popped value must be cleared from spare capacity and
// an evicted value must not remain before a subslice. Those are properties of a
// history, not details every editing operation should remember.
type editorHistory struct {
	undo, redo []editorState
	typist     weak.Pointer[Editor]
}

func (h *editorHistory) clear() {
	h.typist = weak.Pointer[Editor]{}
	clear(h.undo)
	clear(h.redo)
	h.undo = nil
	h.redo = nil
}

func (h *editorHistory) canBack() bool    { return len(h.undo) > 0 }
func (h *editorHistory) canForward() bool { return len(h.redo) > 0 }

func (h *editorHistory) record(state editorState) {
	h.typist = weak.Pointer[Editor]{}
	h.undo = pushEditorState(h.undo, state)
	clear(h.redo)
	h.redo = nil
}

// A typing run has one undo snapshot even though every keystroke publishes a
// change. Retain the combined replaced interval, not an ever-growing edit log.
func (h *editorHistory) extend(change ChangeMap) {
	if len(h.undo) == 0 {
		return
	}
	span := &h.undo[len(h.undo)-1].change
	if span.After == 0 {
		*span = change
		return
	}
	oldEnd := span.OldEnd + max(0, change.OldEnd-span.NewEnd)
	newEnd := change.NewEnd + max(0, span.NewEnd-change.OldEnd)
	span.Start = min(span.Start, change.Start)
	span.OldEnd, span.NewEnd, span.After = oldEnd, newEnd, change.After
}

func (c ChangeMap) inverse() ChangeMap {
	return ChangeMap{Start: c.Start, OldEnd: c.NewEnd, NewEnd: c.OldEnd}
}

func (h *editorHistory) back(current editorState) (editorState, bool) {
	h.typist = weak.Pointer[Editor]{}
	previous, ok := popEditorState(&h.undo)
	if !ok {
		return editorState{}, false
	}
	current.change = previous.change.inverse()
	h.redo = pushEditorState(h.redo, current)
	return previous, true
}

func (h *editorHistory) forward(current editorState) (editorState, bool) {
	h.typist = weak.Pointer[Editor]{}
	next, ok := popEditorState(&h.redo)
	if !ok {
		return editorState{}, false
	}
	current.change = next.change.inverse()
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

func (h *editorHistory) endTyping(view *Editor) {
	if h.typist.Value() == view {
		h.typist = weak.Pointer[Editor]{}
	}
}
