package headless

import "strings"

// editorKillRingLimit bounds text retained outside the document. A kill ring is
// clipboard-like history, not application history: sixteen distinct kills are enough
// to recover recent text without letting a long-lived editor grow with every cut.
const editorKillRingLimit = 16

// editorKillRing owns recently killed text, newest first.
//
// It knows the ring's storage and accumulation rules; the editor knows whether two
// actions were consecutive and which side of the newest entry a deletion came from.
type editorKillRing struct {
	entries []string
}

func (r *editorKillRing) add(text string, prepend, join bool) {
	if text == "" {
		return
	}
	text = strings.Clone(text)
	if join && len(r.entries) > 0 {
		if prepend {
			r.entries[0] = text + r.entries[0]
		} else {
			r.entries[0] += text
		}
		return
	}

	if len(r.entries) < editorKillRingLimit {
		r.entries = append(r.entries, "")
	}
	copy(r.entries[1:], r.entries[:len(r.entries)-1])
	r.entries[0] = text
}

func (r *editorKillRing) newest() (string, bool) {
	if len(r.entries) == 0 {
		return "", false
	}
	return r.entries[0], true
}

func (r *editorKillRing) older(at int) (text string, next int, ok bool) {
	if len(r.entries) < 2 || at < 0 || at >= len(r.entries) {
		return "", 0, false
	}
	next = (at + 1) % len(r.entries)
	return r.entries[next], next, true
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
