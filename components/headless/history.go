package headless

import (
	"slices"

	"github.com/Tangerg/oolong/core/fuzzy"
)

// History owns a bounded sequence and a walk that restores its starting draft.
// The caller decides which values to record, including whether empty values count.
//
// The zero value uses assignment to retain values and keeps consecutive duplicates.
// Use [NewHistory] to configure copying and equality. A History must not be copied
// after first use or accessed concurrently.
type History[T any] struct {
	noCopy noCopy

	entries []T
	// at counts from the end: zero is the draft, one the newest entry. Shrinking
	// may leave it one past the oldest retained entry, so Forward reaches that
	// entry before returning to newer ones and finally the draft.
	at    int
	draft T
	limit int
	clone func(T) T
	equal func(T, T) bool
}

// HistoryConfig fixes value ownership and duplicate identity for a history's lifetime.
type HistoryConfig[T any] struct {
	// Limit bounds retained entries. Zero uses [DefaultHistoryLimit]; negative panics.
	Limit int
	// Clone must preserve the value while copying all mutable referenced data. It
	// must be observationally pure, without modifying its argument or external state.
	// Nil uses assignment, suitable for immutable values. It is used when retaining
	// inputs and exposing retained entries.
	Clone func(T) T
	// Equal suppresses consecutive duplicates. It borrows its arguments read-only;
	// nil keeps every entry. [Equal] compares ordinary comparable values.
	Equal func(T, T) bool
}

// NewHistory constructs an empty history. Copying and equality cannot change after
// construction, so previously retained entries keep the same ownership contract.
func NewHistory[T any](config HistoryConfig[T]) *History[T] {
	h := &History[T]{clone: config.Clone, equal: config.Equal}
	h.SetLimit(config.Limit)
	return h
}

// DefaultHistoryLimit applies when no explicit capacity is configured.
const DefaultHistoryLimit = 1000

// SetLimit drops the oldest excess entries. Zero restores [DefaultHistoryLimit];
// a negative limit panics. If the current entry is dropped, the next Forward reaches
// the oldest retained entry without losing the original draft.
func (h *History[T]) SetLimit(n int) {
	if n < 0 {
		panic("headless: history limit cannot be negative")
	}
	h.limit = n
	h.trim()
}

// Limit reports the effective entry limit, including the default for zero.
func (h *History[T]) Limit() int {
	if h.limit == 0 {
		return DefaultHistoryLimit
	}
	return h.limit
}

// Add ends the current walk and records a snapshot of value, unless Equal matches
// the newest entry. Nonconsecutive duplicates and zero values are retained.
func (h *History[T]) Add(value T) {
	var zero T
	h.at, h.draft = 0, zero
	if len(h.entries) > 0 && h.equal != nil && h.equal(h.entries[len(h.entries)-1], value) {
		return
	}
	h.entries = append(h.entries, h.snapshot(value))
	h.trim()
}

func (h *History[T]) snapshot(value T) T {
	if h.clone != nil {
		return h.clone(value)
	}
	return value
}

func (h *History[T]) trim() {
	limit := h.Limit()
	if len(h.entries) > limit {
		dropped := len(h.entries) - limit
		clear(h.entries[:dropped])
		h.entries = trim(h.entries[dropped:])
		if h.at > len(h.entries) {
			h.at = len(h.entries) + 1
		}
	}
}

// Len reports the number of retained entries, excluding the draft.
func (h *History[T]) Len() int { return len(h.entries) }

// At returns a snapshot of the entry n steps back, one being the newest.
// Out-of-range steps return the zero value and false without changing the walk.
func (h *History[T]) At(n int) (T, bool) {
	if n < 1 || n > len(h.entries) {
		var zero T
		return zero, false
	}
	return h.snapshot(h.entries[len(h.entries)-n]), true
}

// Walking reports whether Back has moved away from the draft.
func (h *History[T]) Walking() bool { return h.at > 0 }

// Back snapshots current as the draft only on the first successful step. Later
// steps return older entry snapshots; the oldest boundary returns zero and false.
func (h *History[T]) Back(current T) (T, bool) {
	if h.at >= len(h.entries) {
		var zero T
		return zero, false
	}
	if h.at == 0 {
		h.draft = h.snapshot(current)
	}
	h.at++
	return h.At(h.at)
}

// Forward returns a snapshot of the next newer entry. Passing the newest transfers
// the saved draft back to the caller and ends the walk; an idle walk returns false.
func (h *History[T]) Forward() (T, bool) {
	if h.at == 0 {
		var zero T
		return zero, false
	}
	h.at--
	if h.at == 0 {
		var zero T
		draft := h.draft
		h.draft = zero
		return draft, true
	}
	return h.At(h.at)
}

// Cancel ends a walk and transfers its saved draft back to the caller. It returns
// the zero value and false when no walk is active.
func (h *History[T]) Cancel() (T, bool) {
	if h.at == 0 {
		var zero T
		return zero, false
	}
	h.at = 0
	var zero T
	draft := h.draft
	h.draft = zero
	return draft, true
}

// Recall returns matching entry snapshots newest first, without changing the walk.
// text borrows each entry read-only and supplies its search text; it does not define
// identity. An empty query returns everything without calling text. A nonempty
// query with nil text matches nothing. Match offsets address the projected text.
func (h *History[T]) Recall(query string, text func(T) string) []Recalled[T] {
	if query != "" && text == nil {
		return nil
	}
	out := make([]Recalled[T], 0, len(h.entries))
	for i, entry := range slices.Backward(h.entries) {
		var match fuzzy.Match
		if query != "" {
			var ok bool
			match, ok = fuzzy.Score(query, text(entry))
			if !ok {
				continue
			}
		}
		out = append(out, Recalled[T]{Entry: h.snapshot(entry), Step: len(h.entries) - i, At: match.At})
	}
	return out
}

// Recalled pairs an owned entry snapshot with its search result.
type Recalled[T any] struct {
	Entry T
	// Step addresses [History.At], one being the newest retained entry.
	Step int
	// At holds byte offsets in the projected search text for highlighting.
	At []int
}
