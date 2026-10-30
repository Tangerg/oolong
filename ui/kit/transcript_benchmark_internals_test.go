package kit

import (
	"fmt"
	"testing"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/grid"
)

// BenchmarkTranscriptMarkCommittedHistory asks the performance-model question
// directly: does drawing a fixed viewport become more expensive as search matches
// accumulate entirely before it?
func BenchmarkTranscriptMarkCommittedHistory(b *testing.B) {
	for _, count := range []int{1_000, 100_000} {
		b.Run(fmt.Sprintf("matches=%d", count), func(b *testing.B) {
			matches := make([]base.Match, count)
			for i := range matches {
				matches[i] = base.Match{Row: i, Spans: []base.Span{{Width: 1}}}
			}
			transcript, err := NewTranscript(TranscriptConfig{})
			if err != nil {
				b.Fatal(err)
			}
			transcript.Controller().SetMatches(matches)
			view := grid.NewSurface(80, 24).View()
			from := count + 100

			b.ResetTimer()
			for range b.N {
				transcript.mark(view, from, transcript.Controller().Layout())
			}
		})
	}
}

func TestVisibleMatchesIncludesTheOneCrossingTheViewportEdge(t *testing.T) {
	matches := []base.Match{
		{Row: 2, Spans: make([]base.Span, 3)}, // reaches row 4
		{Row: 6, Spans: []base.Span{{Width: 1}}},
		{Row: 20, Spans: []base.Span{{Width: 1}}},
	}
	transcript, err := base.NewTranscript(base.TranscriptConfig{})
	if err != nil {
		t.Fatal(err)
	}
	transcript.SetMatches(matches)
	start, visible := transcript.MatchesIn(4, 10)
	if start != 0 || len(visible) != 2 {
		t.Fatalf("visible range = [%d:%d], want [0:2]", start, start+len(visible))
	}
}
