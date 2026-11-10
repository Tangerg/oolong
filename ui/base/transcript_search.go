package base

import (
	"errors"
	"fmt"
	"math"
	"slices"
	"sort"
)

// ErrStaleMatches rejects physical matches from a different transcript, content revision or width.
var ErrStaleMatches = errors.New("base: search result does not match the transcript projection")

// ErrInvalidMatches rejects unordered matches, out-of-range row spans and invalid cell ranges.
var ErrInvalidMatches = errors.New("base: invalid transcript search matches")

type transcriptSearch struct {
	source  transcriptLayoutKey
	matches []Match
	// Row starts alone cannot locate earlier matches that overlap a later viewport.
	prefixEnds []int
	current    int
}

func newTranscriptSearch(source TranscriptLayout, matches []Match) (transcriptSearch, error) {
	search := transcriptSearch{source: source.key, current: -1}
	previousRow, end := source.StartRow(), source.EndRow()
	for i, match := range matches {
		if match.Row < previousRow || match.Row >= end {
			return transcriptSearch{}, fmt.Errorf("%w: match %d has an out-of-order or out-of-range row", ErrInvalidMatches, i)
		}
		if len(match.Spans) == 0 || len(match.Spans) > end-match.Row {
			return transcriptSearch{}, fmt.Errorf("%w: match %d has an empty or out-of-range row span", ErrInvalidMatches, i)
		}
		for j, span := range match.Spans {
			if span.Col < 0 || span.Width < 0 || span.Width > math.MaxInt-span.Col {
				return transcriptSearch{}, fmt.Errorf("%w: match %d span %d has an invalid cell range", ErrInvalidMatches, i, j)
			}
		}
		previousRow = match.Row
	}
	search.matches = cloneMatches(matches)
	search.prefixEnds = make([]int, len(matches))
	through := source.StartRow()
	for i, match := range matches {
		through = max(through, match.Row+len(match.Spans))
		search.prefixEnds[i] = through
	}
	return search, nil
}

func cloneMatches(matches []Match) []Match {
	cloned := slices.Clone(matches)
	for i := range cloned {
		cloned[i].Spans = slices.Clone(cloned[i].Spans)
	}
	return cloned
}

// Matches returns a detached copy of the current search projection.
func (t *Transcript) Matches() []Match {
	if t.search.source != t.Layout().key {
		return nil
	}
	return cloneMatches(t.search.matches)
}

// MatchesIn preserves result indices while copying the smallest contiguous range
// containing every match that intersects [from, to). Its cost is logarithmic in
// the result count plus the returned range and its spans.
func (t *Transcript) MatchesIn(source TranscriptLayout, from, to int) (first int, matches []Match) {
	search := &t.search
	if search.source != source.key || len(search.matches) == 0 || from >= to {
		return 0, nil
	}
	first = sort.Search(len(search.matches), func(i int) bool { return search.prefixEnds[i] > from })
	last := sort.Search(len(search.matches), func(i int) bool { return search.matches[i].Row >= to })
	return first, cloneMatches(search.matches[first:max(first, last)])
}

// CurrentMatch returns the revealed match index, or -1 when no match is current.
func (t *Transcript) CurrentMatch(source TranscriptLayout) int {
	if t.search.source != source.key || len(t.search.matches) == 0 {
		return -1
	}
	return t.search.current
}

// SetMatches accepts detached physical matches in nondecreasing row order, with
// at least one span per match inside source's row space and nonnegative cell
// ranges. Overlapping matches and zero-width spans are valid. A foreign or stale
// source returns ErrStaleMatches; malformed matches return ErrInvalidMatches.
// Cell ranges may extend beyond the viewport and are clipped during drawing.
// Rejection leaves the previously accepted search state unchanged.
func (t *Transcript) SetMatches(source TranscriptLayout, matches []Match) error {
	t.assertMutable()
	if source.key.owner != t.namespace || source.key.revision != t.revision || source.key != t.Layout().key {
		return ErrStaleMatches
	}
	search, err := newTranscriptSearch(source, matches)
	if err != nil {
		return err
	}
	t.search = search
	t.element.Invalidate()
	return nil
}

// RevealMatch selects a valid match and asks the scroll owner to reveal its complete row span.
func (t *Transcript) RevealMatch(index int) bool {
	t.assertMutable()
	if t.search.source != t.Layout().key || index < 0 || index >= len(t.search.matches) {
		return false
	}
	match := t.search.matches[index]
	t.search.current = index
	first := match.Row - t.Layout().StartRow()
	t.scroll.requestReveal(first, first+len(match.Spans)-1)
	t.element.Invalidate()
	return true
}
