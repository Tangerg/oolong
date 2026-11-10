// Package fuzzy returns original UTF-8 byte offsets so consumers can highlight
// matches without translating coordinates. Matching favors word starts and runs
// of adjacent characters, with case used to distinguish otherwise equal matches.
package fuzzy

import (
	"cmp"
	"iter"
	"slices"
	"unicode"
	"unicode/utf8"
)

// What a match is worth. The numbers matter only relative to each other: a word-start
// match is worth twice a plain one, a run of two adjacent characters beats the same
// two characters a word apart, and a gap costs enough to break ties without being
// able to push a real match below a worse one.
const (
	perCharacter     = 16
	bonusWordStart   = 32
	bonusConsecutive = 24
	bonusSameCase    = 8
	penaltyGap       = 3

	// bonusStart is what beginning the candidate is worth on top of beginning a word.
	// Without it "status_line" and "test_status" answer "st" equally well, and they do
	// not: the thing that starts with what was typed is the thing that was meant.
	bonusStart = 24
)

type Match struct {
	// Score is meaningful only against other scores for the same pattern; higher
	// is better. Whether there was a match at all is Score's boolean result, not
	// this value: an empty pattern matches with a score of zero.
	Score int
	// At is the byte offset in the candidate of each character the pattern matched,
	// ascending. An offset can fall inside a grapheme cluster — a pattern character
	// can match a combining mark — so something highlighting the matches should ask
	// whether a cluster contains an offset rather than begins at one.
	At []int
}

// Score matches pattern against candidate, case-insensitively, and reports whether
// it matched at all. An empty pattern matches everything, with nothing highlighted.
//
// It is not a full alignment search. Where a pattern character could bind to several
// places this tries the first and every one that begins a word, which is what makes
// "st" find the "status" in "test_status" rather than the "st" in "test". A better
// placement with no word boundary to hint at it can still be missed: candidates are
// palette-sized and the alternative is a scoring matrix per candidate.
func Score(pattern, candidate string) (Match, bool) {
	if pattern == "" {
		return Match{}, true
	}
	firstRune, _ := utf8.DecodeRuneInString(pattern)
	var needle []rune
	var points []matchPoint
	var best Match
	for first := range anchors(candidate, firstRune) {
		if points == nil {
			length := utf8.RuneCountInString(pattern)
			if length > len(candidate)-first.offset {
				break
			}
			needle = []rune(pattern)
			points = make([]matchPoint, length)
		}
		points[0] = first
		score, ok := extend(candidate, needle, points)
		if !ok {
			// Every later anchor starts inside this failed suffix.
			break
		}
		if score <= best.Score {
			continue
		}
		if best.At == nil {
			best.At = make([]int, len(points))
		}
		best.Score = score
		for i, point := range points {
			best.At[i] = point.offset
		}
	}
	return best, best.Score != 0
}

type matchPoint struct {
	offset, index int
	char          rune
	wordStart     bool
}

func anchors(candidate string, first rune) iter.Seq[matchPoint] {
	return func(yield func(matchPoint) bool) {
		prev, index, found := rune(-1), 0, false
		for offset, char := range candidate {
			wordStart := opensWord(prev)
			if (!found || wordStart) && folds(char, first) {
				found = true
				if !yield(matchPoint{offset, index, char, wordStart}) {
					return
				}
			}
			prev = char
			index++
		}
	}
}

// Advancing the anchor can only move each greedy match forward. Reuse the
// previous suffix positions until a preceding match overtakes them.
func extend(candidate string, pattern []rune, points []matchPoint) (int, bool) {
	score := 0
	for i, want := range pattern {
		if i > 0 {
			prev := points[i-1]
			if points[i].offset <= prev.offset {
				next, ok := prev.next(candidate, want)
				if !ok {
					return 0, false
				}
				points[i] = next
			}
			gap := points[i].index - prev.index - 1
			if gap == 0 {
				score += bonusConsecutive
			} else {
				score -= penaltyGap * gap
			}
		}
		p := points[i]
		score += perCharacter
		if p.wordStart {
			score += bonusWordStart
		}
		if p.offset == 0 {
			score += bonusStart
		}
		if p.char == want {
			score += bonusSameCase
		}
	}
	return max(score, 1), true
}

func (p matchPoint) next(candidate string, want rune) (matchPoint, bool) {
	prev, index := p.char, p.index
	for delta, char := range candidate[p.offset:] {
		if delta == 0 {
			continue
		}
		index++
		if folds(char, want) {
			return matchPoint{p.offset + delta, index, char, opensWord(prev)}, true
		}
		prev = char
	}
	return matchPoint{}, false
}

// folds reports whether two characters are the same under Unicode's simple case
// folding. Lowercasing both is not equivalent: some letters, such as the two forms
// of Greek sigma, share a fold but have different lowercase forms.
func folds(a, b rune) bool {
	if a == b {
		return true
	}
	for folded := unicode.SimpleFold(a); folded != a; folded = unicode.SimpleFold(folded) {
		if folded == b {
			return true
		}
	}
	return false
}

// opensWord reports whether a character makes what follows it the start of a word.
//
// Separators only, not a case change: treating a capital as a word start would score
// "SQL" as three words, and the point of the bonus is to find the beginning of
// something a person would name.
func opensWord(prev rune) bool {
	switch prev {
	case ' ', '\t', '_', '-', '.', '/', ':':
		return true
	}
	return prev < 0
}

// Ranked uses an index so callers retain their item identity without matching
// the resulting text back to their records.
type Ranked struct {
	Index int
	Match Match
}

// Filter scores every candidate and returns those that matched, best first.
//
// Candidates that score the same stay in the order they were given, so a caller that
// sorted its items meaningfully keeps that order for ties — which is where a caller's
// own idea of importance belongs.
func Filter(pattern string, candidates []string) []Ranked {
	return FilterFunc(pattern, candidates, func(s string) string { return s })
}

// FilterFunc ranks caller-owned items directly so a parallel string slice
// cannot drift from the records addressed by Ranked.Index.
func FilterFunc[T any](pattern string, items []T, text func(T) string) []Ranked {
	out := make([]Ranked, 0, len(items))
	for i, item := range items {
		if m, ok := Score(pattern, text(item)); ok {
			out = append(out, Ranked{Index: i, Match: m})
		}
	}
	slices.SortStableFunc(out, func(a, b Ranked) int {
		return cmp.Compare(b.Match.Score, a.Match.Score)
	})
	return out
}
