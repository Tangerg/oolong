package headless

import (
	"slices"
	"strings"

	"github.com/Tangerg/oolong/core/fuzzy"
)

// History is what the user typed before, and a place in it.
//
// # The draft
//
// Walking back through history has to be undoable, and the thing that gets lost is
// what the user had already typed: they are half way through a line, press up to
// check something, and press down again expecting their line back. So the first step
// backwards keeps the draft, and coming forward past the newest entry gives it back.
// Nothing else in a prompt is as annoying to lose, because it is the only text the
// user cannot get again by scrolling.
//
// # What is not kept
//
// Consecutive duplicates, and empty lines. Somebody who runs the same thing twice has
// not made two entries worth stepping through, and a blank line was not an entry at
// all. Duplicates that are not consecutive are kept, because the order tells the
// truth about what happened.
//
// The zero value is an empty history.
type History struct {
	entries []string
	// at is where the walk has got to, counted from the end: zero is the draft, one
	// is the newest entry. It is not an index into the slice, so that entries added
	// during a walk cannot move the place.
	at int
	// draft is what was being typed when the walk began.
	draft string
	// maxEntries bounds the list. Zero uses DefaultHistory.
	maxEntries int
}

// DefaultHistory is how many entries a history keeps when it is not told.
const DefaultHistory = 1000

// Limit sets how many entries to keep, dropping the oldest if there are already more.
func (h *History) Limit(n int) {
	h.maxEntries = max(n, 0)
	h.trim()
}

// Add records a line, unless it is empty or the same as the newest entry.
//
// It also ends any walk in progress, which is what submitting a line means: whatever
// the user was stepping through, they have now said something, and the next press of
// up starts again from the end.
func (h *History) Add(line string) {
	h.at, h.draft = 0, ""
	if strings.TrimSpace(line) == "" {
		return
	}
	if len(h.entries) > 0 && h.entries[len(h.entries)-1] == line {
		return
	}
	h.entries = append(h.entries, line)
	h.trim()
}

func (h *History) trim() {
	limit := h.maxEntries
	if limit == 0 {
		limit = DefaultHistory
	}
	if len(h.entries) > limit {
		h.entries = slices.Clone(h.entries[len(h.entries)-limit:])
	}
}

// Len is how many entries are kept.
func (h *History) Len() int { return len(h.entries) }

// At is the entry n steps back from the end, one being the newest.
func (h *History) At(n int) (string, bool) {
	if n < 1 || n > len(h.entries) {
		return "", false
	}
	return h.entries[len(h.entries)-n], true
}

// Walking reports whether a walk through the history is in progress.
func (h *History) Walking() bool { return h.at > 0 }

// Back steps one entry further into the past, and reports what should now be in the
// field.
//
// current is what is in the field now, which is kept as the draft on the first step so
// that [History.Forward] can give it back. It reports false at the oldest entry, so a
// caller can leave the field alone rather than clearing it.
func (h *History) Back(current string) (string, bool) {
	if h.at >= len(h.entries) {
		return "", false
	}
	if h.at == 0 {
		h.draft = current
	}
	h.at++
	entry, _ := h.At(h.at)
	return entry, true
}

// Forward steps one entry towards the present.
//
// Stepping forward past the newest entry gives back the draft the walk began with,
// which is the whole reason the draft is kept.
func (h *History) Forward() (string, bool) {
	if h.at == 0 {
		return "", false
	}
	h.at--
	if h.at == 0 {
		draft := h.draft
		h.draft = ""
		return draft, true
	}
	entry, _ := h.At(h.at)
	return entry, true
}

// Cancel abandons a walk and reports the draft it began with, if there was one.
func (h *History) Cancel() (string, bool) {
	if h.at == 0 {
		return "", false
	}
	h.at = 0
	draft := h.draft
	h.draft = ""
	return draft, true
}

// Recall is the entries a query matches, newest first.
//
// The order is the point. Scoring alone would put the best-matching line first and
// bury the one from a minute ago behind six from last week, and "the one I ran
// recently" is what somebody searching their own history is nearly always after — so
// matches are found by score and then shown newest first.
func (h *History) Recall(query string) []Recalled {
	if query == "" {
		out := make([]Recalled, 0, len(h.entries))
		for i := len(h.entries) - 1; i >= 0; i-- {
			out = append(out, Recalled{Entry: h.entries[i], Step: len(h.entries) - i})
		}
		return out
	}
	ranked := fuzzy.Filter(query, h.entries)
	out := make([]Recalled, 0, len(ranked))
	for _, r := range ranked {
		out = append(out, Recalled{
			Entry: h.entries[r.Index],
			Step:  len(h.entries) - r.Index,
			At:    r.Match.At,
		})
	}
	slices.SortStableFunc(out, func(a, b Recalled) int { return a.Step - b.Step })
	return out
}

// Recalled is one entry a search of the history turned up.
type Recalled struct {
	// Entry is the line.
	Entry string
	// Step is how far back it is, one being the newest, so a caller can jump to it
	// with [History.At].
	Step int
	// At is the byte offsets of the query's characters within the entry, for
	// underlining them.
	At []int
}
