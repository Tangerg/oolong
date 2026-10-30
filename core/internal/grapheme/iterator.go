// Package grapheme keeps malformed UTF-8 outside Unicode segmentation so corrupt
// bytes cannot consume adjacent text or change its original byte coordinates.
package grapheme

import (
	"unicode/utf8"

	"github.com/clipperhouse/uax29/v2/graphemes"
)

// A small first window keeps cursor queries independent of the unread tail.
// Growing by the retained candidate's size bounds rescans of long clusters.
const initialValidationBytes = 64

// Iterator retains malformed bytes as separate clusters. Removing or replacing
// them before segmentation would join text across corruption or move byte offsets.
type Iterator struct {
	source       string
	clusters     graphemes.Iterator[string]
	windowStart  int
	validatedEnd int
	runEnded     bool
	start        int
	end          int
}

// New defers validation so consumers that stop early do not scan the complete input.
func New(s string) Iterator {
	return Iterator{source: s, clusters: *graphemes.FromString("")}
}

// Next retains a provisional tail because more input can still change its boundary.
func (i *Iterator) Next() bool {
	if i.end == len(i.source) {
		return false
	}
	i.start = i.end
	if i.end == i.validatedEnd {
		i.windowStart = i.end
		i.runEnded = false
		i.validateMore(initialValidationBytes)
		if i.validatedEnd == i.end {
			i.end++
			i.validatedEnd = i.end
			return true
		}
		i.clusters.SetText(i.source[i.windowStart:i.validatedEnd])
	}
	for {
		i.clusters.Next()
		end := i.windowStart + i.clusters.End()
		if end < i.validatedEnd || i.runEnded {
			i.end = end
			return true
		}
		// More input can extend the window's last cluster. Only uax29 can
		// confirm that candidate's boundary, so it remains private until then.
		i.windowStart = i.start
		i.validateMore(max(initialValidationBytes, i.validatedEnd-i.start))
		i.clusters.SetText(i.source[i.windowStart:i.validatedEnd])
	}
}

// Start counts original bytes so appearance can follow the cluster's source span.
func (i *Iterator) Start() int { return i.start }

// End counts original bytes so consumers can slice the source without normalization.
func (i *Iterator) End() int { return i.end }

// Value borrows the original bytes so iteration neither allocates nor rewrites input.
func (i *Iterator) Value() string { return i.source[i.start:i.end] }

func (i *Iterator) validateMore(budget int) {
	limit := i.validatedEnd + min(budget, len(i.source)-i.validatedEnd)
	for i.validatedEnd < limit {
		r, size := utf8.DecodeRuneInString(i.source[i.validatedEnd:])
		if r == utf8.RuneError && size == 1 {
			i.runEnded = true
			return
		}
		i.validatedEnd += size
	}
	i.runEnded = i.validatedEnd == len(i.source)
}
