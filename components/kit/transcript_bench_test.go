package kit

import (
	"fmt"
	"testing"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/core/grid"
)

// BenchmarkTranscriptMarkCommittedHistory asks the performance-model question
// directly: does drawing a fixed viewport become more expensive as search matches
// accumulate entirely before it?
func BenchmarkTranscriptMarkCommittedHistory(b *testing.B) {
	for _, count := range []int{1_000, 100_000} {
		b.Run(fmt.Sprintf("matches=%d", count), func(b *testing.B) {
			matches := make([]headless.Match, count)
			for i := range matches {
				matches[i] = headless.Match{Row: i, Spans: []headless.Span{{Width: 1}}}
			}
			transcript := Transcript{Matches: matches, Current: -1}
			view := grid.NewSurface(80, 24).View()
			from := count + 100

			b.ResetTimer()
			for range b.N {
				transcript.mark(view, from)
			}
		})
	}
}

func TestVisibleMatchesIncludesTheOneCrossingTheViewportEdge(t *testing.T) {
	matches := []headless.Match{
		{Row: 2, Spans: make([]headless.Span, 3)}, // reaches row 4
		{Row: 6, Spans: []headless.Span{{Width: 1}}},
		{Row: 20, Spans: []headless.Span{{Width: 1}}},
	}
	start, end := visibleMatches(matches, 4, 10)
	if start != 0 || end != 2 {
		t.Fatalf("visible range = [%d:%d], want [0:2]", start, end)
	}
}
