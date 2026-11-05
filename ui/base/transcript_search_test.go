package base_test

import (
	"errors"
	"math"
	"reflect"
	"testing"

	"github.com/Tangerg/oolong/ui/base"
)

func TestTranscriptSearchWindowIncludesEveryEarlierOverlappingMatch(t *testing.T) {
	transcript := transcriptModel(t, base.TranscriptConfig{})
	transcriptAdd(t, transcript, &plain{lines: 6})
	root := transcriptScene(t, transcript)
	transcriptCommit(t, root, 20, 3, 1)
	layout := transcript.Layout()
	matches := []base.Match{
		{Row: 0, Spans: []base.Span{{Width: 1}, {Width: 1}, {Width: 1}, {Width: 1}, {Width: 1}}},
		{Row: 1, Spans: []base.Span{{Col: 2, Width: 1}}},
		{Row: 2, Spans: []base.Span{{Col: 4, Width: 1}, {Col: 4, Width: 1}, {Col: 4, Width: 1}}},
		{Row: 5, Spans: []base.Span{{Col: 6, Width: 1}}},
	}
	if err := transcript.SetMatches(layout, matches); err != nil {
		t.Fatal(err)
	}
	if !transcript.RevealMatch(2) {
		t.Fatal("overlapping match was not revealable")
	}
	first, visible := transcript.MatchesIn(layout, 4, 5)
	if first != 0 || !reflect.DeepEqual(visible, matches[:3]) {
		t.Fatalf("window lost an earlier overlapping result: first=%d matches=%+v", first, visible)
	}
	if transcript.CurrentMatch(layout) != 2 {
		t.Fatal("window projection changed the selected match identity")
	}
}

func TestTranscriptRejectsInvalidSearchMatchesBeforePublication(t *testing.T) {
	for _, test := range []struct {
		name    string
		matches []base.Match
	}{
		{name: "unsorted", matches: []base.Match{{Row: 2, Spans: []base.Span{{Width: 1}}}, {Row: 1, Spans: []base.Span{{Width: 1}}}}},
		{name: "negative row", matches: []base.Match{{Row: -1, Spans: []base.Span{{Width: 1}}}}},
		{name: "row after content", matches: []base.Match{{Row: 3, Spans: []base.Span{{Width: 1}}}}},
		{name: "span after content", matches: []base.Match{{Row: 2, Spans: []base.Span{{Width: 1}, {Width: 1}}}}},
		{name: "no spans", matches: []base.Match{{Row: 1}}},
		{name: "negative column", matches: []base.Match{{Row: 1, Spans: []base.Span{{Col: -1, Width: 1}}}}},
		{name: "negative width", matches: []base.Match{{Row: 1, Spans: []base.Span{{Width: -1}}}}},
		{name: "column overflow", matches: []base.Match{{Row: 1, Spans: []base.Span{{Col: math.MaxInt, Width: 1}}}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			transcript := transcriptModel(t, base.TranscriptConfig{})
			transcriptAdd(t, transcript, &plain{lines: 3})
			root := transcriptScene(t, transcript)
			transcriptCommit(t, root, 20, 2, 1)
			layout := transcript.Layout()
			previous := []base.Match{{Row: 1, Spans: []base.Span{{Col: 2, Width: 1}}}}
			if err := transcript.SetMatches(layout, previous); err != nil {
				t.Fatal(err)
			}
			transcript.RevealMatch(0)
			changed := root.Changes()
			if err := transcript.SetMatches(layout, test.matches); !errors.Is(err, base.ErrInvalidMatches) {
				t.Fatalf("invalid result error = %v, want ErrInvalidMatches", err)
			}
			if !reflect.DeepEqual(transcript.Matches(), previous) || transcript.CurrentMatch(layout) != 0 {
				t.Fatal("rejected result replaced the previously published search state")
			}
			select {
			case <-changed:
				t.Fatal("rejected result published a scene change")
			default:
			}
		})
	}
}
