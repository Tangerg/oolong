package base

import (
	"slices"
	"weak"

	"github.com/Tangerg/oolong/ui/core/text"
)

const maxUndo = 200

// Replaying the initiating view restores its caret; peers use change instead.
// Marks preserve identities that plain text cannot reconstruct.
type editSnapshot struct {
	view   weak.Pointer[Editor]
	change ChangeMap
	lines  []string
	marks  []text.Mark
	caret  Caret
}

// A discarded snapshot must release its text and marks even when the stack keeps
// spare capacity. Weak view references prevent undo from extending a view's lifetime.
type editHistory struct {
	undo, redo []editSnapshot
	typist     weak.Pointer[Editor]
}

func (h *editHistory) clear() {
	h.typist = weak.Pointer[Editor]{}
	clear(h.undo)
	clear(h.redo)
	h.undo = nil
	h.redo = nil
}

func (h *editHistory) canBack() bool    { return len(h.undo) > 0 }
func (h *editHistory) canForward() bool { return len(h.redo) > 0 }

func (h *editHistory) record(state editSnapshot) {
	h.typist = weak.Pointer[Editor]{}
	h.undo = pushEditSnapshot(h.undo, state)
	clear(h.redo)
	h.redo = nil
}

// A typing run has one undo snapshot even though every keystroke publishes a
// change. Retain the combined replaced interval, not an ever-growing edit log.
func (h *editHistory) extend(change ChangeMap) {
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

func (h *editHistory) back(current editSnapshot) (editSnapshot, bool) {
	return h.transfer(&h.undo, &h.redo, current)
}

func (h *editHistory) forward(current editSnapshot) (editSnapshot, bool) {
	return h.transfer(&h.redo, &h.undo, current)
}

func (h *editHistory) transfer(from, to *[]editSnapshot, current editSnapshot) (editSnapshot, bool) {
	h.typist = weak.Pointer[Editor]{}
	saved, ok := popEditSnapshot(from)
	if !ok {
		return editSnapshot{}, false
	}
	// Replaying another view's edit maps every caret through the document change.
	// Its inverse must not acquire the replaying view's unrelated caret.
	if current.view != saved.view {
		current.view = weak.Pointer[Editor]{}
	}
	current.change = saved.change.inverse()
	*to = pushEditSnapshot(*to, current)
	return saved, true
}

func pushEditSnapshot(stack []editSnapshot, state editSnapshot) []editSnapshot {
	stack = append(stack, state)
	if len(stack) <= maxUndo {
		return stack
	}
	drop := len(stack) - maxUndo
	copy(stack, stack[drop:])
	clear(stack[len(stack)-drop:])
	return stack[:maxUndo]
}

func popEditSnapshot(stack *[]editSnapshot) (editSnapshot, bool) {
	if len(*stack) == 0 {
		return editSnapshot{}, false
	}
	last := len(*stack) - 1
	state := (*stack)[last]
	(*stack)[last] = editSnapshot{}
	*stack = (*stack)[:last]
	return state, true
}

func (h *editHistory) endTyping(view *Editor) {
	if h.typist.Value() == view {
		h.typist = weak.Pointer[Editor]{}
	}
}

func (s *EditSession) snapshot(view *Editor) editSnapshot {
	return editSnapshot{
		view: weak.Make(view), lines: slices.Clone(s.lines), marks: slices.Clone(s.marks),
		caret: Caret{Line: view.line, Byte: view.byteOffset},
	}
}

func (s *EditSession) undo(view *Editor) {
	if !s.history.canBack() {
		view.endTyping()
		return
	}
	s.requireRevision()
	view.endTyping()
	previous, _ := s.history.back(s.snapshot(view))
	s.restore(view, previous)
}

func (s *EditSession) redo(view *Editor) {
	if !s.history.canForward() {
		view.endTyping()
		return
	}
	s.requireRevision()
	view.endTyping()
	next, _ := s.history.forward(s.snapshot(view))
	s.restore(view, next)
}

func (s *EditSession) restore(view *Editor, snapshot editSnapshot) {
	changed := !slices.Equal(s.lines, snapshot.lines) || !slices.Equal(s.marks, snapshot.marks)
	before := s.lines
	s.lines, s.marks = slices.Clone(snapshot.lines), slices.Clone(snapshot.marks)
	var restored *Editor
	if snapshot.view.Value() == view {
		view.restoreCaret(snapshot.caret)
		restored = view
	}
	if changed {
		s.commit(restored, before, snapshot.change.inverse(), false)
	}
}

func (s *EditSession) endTyping(view *Editor) {
	s.history.endTyping(view)
}

func (s *EditSession) forgetHistory() {
	s.requireMutable()
	s.history.clear()
}

func (s *EditSession) prepareHistory(view *Editor, policy editHistoryPolicy, changed bool) {
	switch policy {
	case discardEdits:
		s.history.clear()
	case recordEdit:
		if changed {
			s.history.record(s.snapshot(view))
		} else {
			s.history.endTyping(view)
		}
	case coalesceEdit:
		if !changed {
			s.history.endTyping(view)
			return
		}
		if s.history.typist.Value() != view {
			s.history.record(s.snapshot(view))
		}
		s.history.typist = weak.Make(view)
	}
}
