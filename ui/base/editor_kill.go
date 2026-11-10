package base

import (
	"slices"
	"strings"
)

// editorKillRingLimit bounds text retained outside the document. A kill ring is
// clipboard-like history, not application history: sixteen distinct kills are enough
// to recover recent text without letting a long-lived editor grow with every cut.
const editorKillRingLimit = 16

// editorKillRing owns recently killed text, newest first.
//
// It knows the ring's storage and accumulation rules; the editor knows whether two
// actions were consecutive and which side of the newest entry a deletion came from.
type editorKillRing struct {
	entries []editorKill
}

func (r *editorKillRing) add(text string, prepend, join bool) {
	if text == "" {
		return
	}
	if join && len(r.entries) > 0 {
		r.entries[0].join(text, prepend)
		return
	}

	if len(r.entries) < editorKillRingLimit {
		r.entries = append(r.entries, editorKill{})
	}
	copy(r.entries[1:], r.entries[:len(r.entries)-1])
	r.entries[0] = newEditorKill(text)
}

func (r *editorKillRing) newest() (string, bool) {
	if len(r.entries) == 0 {
		return "", false
	}
	return r.entries[0].materialise(), true
}

func (r *editorKillRing) older(at int) (text string, next int, ok bool) {
	if len(r.entries) < 2 || at < 0 || at >= len(r.entries) {
		return "", 0, false
	}
	next = (at + 1) % len(r.entries)
	return r.entries[next].materialise(), next, true
}

// editorKill is one logical cut assembled from consecutive kill actions.
//
// A user can hold the action through an arbitrarily long paragraph. Keeping the
// pieces until the value is actually yanked makes each action copy only the bytes
// it owns; repeatedly rebuilding the complete string would make that ordinary
// interaction quadratic. Prepended pieces are stored in arrival order and read in
// reverse, while appended pieces keep their arrival order.
type editorKill struct {
	value  string
	before []string
	after  []string
	bytes  int
}

func newEditorKill(text string) editorKill {
	text = strings.Clone(text)
	return editorKill{value: text, bytes: len(text)}
}

func (k *editorKill) join(text string, prepend bool) {
	text = strings.Clone(text)
	if prepend {
		k.before = append(k.before, text)
	} else {
		k.after = append(k.after, text)
	}
	k.bytes += len(text)
}

// materialise settles the pieces into one owned value. It also releases their
// slice storage, so reading a kill does not leave both the assembled text and
// every source allocation alive.
func (k *editorKill) materialise() string {
	if len(k.before) == 0 && len(k.after) == 0 {
		return k.value
	}
	var out strings.Builder
	out.Grow(k.bytes)
	for _, text := range slices.Backward(k.before) {
		out.WriteString(text)
	}
	out.WriteString(k.value)
	for _, text := range k.after {
		out.WriteString(text)
	}
	k.value = out.String()
	clear(k.before)
	clear(k.after)
	k.before = nil
	k.after = nil
	return k.value
}

// editorContinuation is what the previous command leaves for the next one to
// continue: a kill that the next kill joins, or a yank that YankPop replaces.
// It is assigned whole, so a yank range never outlives its yank.
type editorContinuation struct {
	kind editorContinuationKind
	// yank is the inserted range and its ring position; set for a yank only.
	yank editorYank
}

type editorContinuationKind uint8

const (
	editorContinuationNone editorContinuationKind = iota
	editorContinuationKill
	editorContinuationYank
)

type editorYank struct {
	start, end Caret
	ring       int
}

// KillToEnd cuts from the cursor to the end of the line, keeping what it cut.
// At the end of a line, the line break itself belongs to the kill.
func (e *Editor) KillToEnd() {
	e.beginViewUpdate()
	defer e.endViewUpdate()
	e.ensure()
	start := Caret{Line: e.line, Byte: e.byteOffset}
	end := Caret{Line: e.line, Byte: len(e.document().lines[e.line])}
	if start == end && end.Line+1 < len(e.document().lines) {
		end = Caret{Line: end.Line + 1}
	}
	e.killRange(start, end, false)
}

// KillToStart cuts from the start of the line to the cursor.
func (e *Editor) KillToStart() {
	e.beginViewUpdate()
	defer e.endViewUpdate()
	e.ensure()
	e.killRange(Caret{Line: e.line}, Caret{Line: e.line, Byte: e.byteOffset}, true)
}

func (e *Editor) killRange(start, end Caret, prepend bool) {
	join := e.continuation.kind == editorContinuationKill
	if start == end {
		e.endTyping()
		return
	}
	killed := e.document().textBetween(start, end)
	e.document().replace(e, editReplacement{start: start, end: end}, func(at Caret, changed bool) {
		e.rememberKill(killed, prepend, join)
		e.finishReplacement(at, changed)
	})
}

// Yank puts back the most recently killed text.
func (e *Editor) Yank() {
	e.beginViewUpdate()
	defer e.endViewUpdate()
	killed, ok := e.kills.newest()
	if !ok {
		e.endTyping()
		return
	}
	e.ensure()
	start, end := Caret{Line: e.line, Byte: e.byteOffset}, Caret{Line: e.line, Byte: e.byteOffset}
	if first, last, selected := e.Selection(); selected {
		start, end = first, last
	}
	e.applyYank(start, end, killed, 0)
}

// YankPop replaces the immediately preceding yank with the next older kill, cycling
// through the bounded ring. Any intervening edit, movement, or selection ends the
// sequence and makes this a no-op.
func (e *Editor) YankPop() {
	e.beginViewUpdate()
	defer e.endViewUpdate()
	previous := e.continuation
	if previous.kind != editorContinuationYank {
		e.endTyping()
		return
	}
	killed, next, ok := e.kills.older(previous.yank.ring)
	if !ok {
		e.endTyping()
		return
	}
	e.applyYank(previous.yank.start, previous.yank.end, killed, next)
}

func (e *Editor) applyYank(start, end Caret, killed string, ring int) {
	e.document().replace(e, editReplacement{start: start, end: end, text: killed}, func(at Caret, changed bool) {
		e.finishReplacement(at, changed)
		e.continuation = editorContinuation{kind: editorContinuationYank, yank: editorYank{start: start, end: at, ring: ring}}
	})
}

func (e *Editor) breakContinuation() {
	e.continuation = editorContinuation{}
}

func (e *Editor) rememberKill(text string, prepend, join bool) {
	e.kills.add(text, prepend, join)
	e.continuation = editorContinuation{kind: editorContinuationKill}
}
