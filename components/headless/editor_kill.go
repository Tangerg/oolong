package headless

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

type editorContinuation uint8

const (
	editorContinuationNone editorContinuation = iota
	editorContinuationKill
	editorContinuationYank
)

type editorYank struct {
	start, end Caret
	ring       int
}

func (e *Editor) breakContinuation() {
	e.continuation = editorContinuationNone
	e.yank = editorYank{}
}

func (e *Editor) rememberKill(text string, prepend, join bool) {
	e.kills.add(text, prepend, join)
	e.continuation = editorContinuationKill
	e.yank = editorYank{}
}
