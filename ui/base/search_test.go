package base_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/text"
)

func TestSearchProjectsLogicalMatchesToVisibleCells(t *testing.T) {
	tests := []struct {
		name, query string
		regex       bool
		rows        []text.Row
		want        []base.Match
	}{
		{name: "occurrences", query: "cat", rows: []text.Row{{Text: "the cat sat"}, {Text: "on the mat"}, {Text: "with a cat"}}, want: []base.Match{{Row: 0, Spans: []base.Span{{Col: 4, Width: 3}}}, {Row: 2, Spans: []base.Span{{Col: 7, Width: 3}}}}},
		{name: "case folding", query: "cat", rows: []text.Row{{Text: "The Cat SAT"}}, want: []base.Match{{Spans: []base.Span{{Col: 4, Width: 3}}}}},
		{name: "folding preserves UTF-8 offsets", query: "cat", rows: []text.Row{{Text: "İstanbul then cat"}}, want: []base.Match{{Spans: []base.Span{{Col: 14, Width: 3}}}}},
		{name: "wide character", query: "cat", rows: []text.Row{{Text: "中文 cat"}}, want: []base.Match{{Spans: []base.Span{{Col: 5, Width: 3}}}}},
		{name: "indent", query: "me", rows: []text.Row{{Text: "find me", Offset: 3}}, want: []base.Match{{Spans: []base.Span{{Col: 8, Width: 2}}}}},
		{name: "soft wrap", query: "quick brown", rows: []text.Row{{Text: "the quick"}, {Text: "brown fox", Joined: true, Gap: " "}}, want: []base.Match{{Spans: []base.Span{{Col: 4, Width: 5}, {Col: 0, Width: 5}}}}},
		{name: "hard break", query: "sat on", rows: []text.Row{{Text: "the cat sat"}, {Text: "on the mat"}}},
		{name: "regex", query: `error \d+`, regex: true, rows: []text.Row{{Text: "error 42 and error 7"}}, want: []base.Match{{Spans: []base.Span{{Col: 0, Width: 8}}}, {Spans: []base.Span{{Col: 13, Width: 7}}}}},
		{name: "no match", query: "absent", rows: []text.Row{{Text: "one"}, {Text: "two"}}},
		{name: "literal", query: "a+b", rows: []text.Row{{Text: "a+b and aab"}}, want: []base.Match{{Spans: []base.Span{{Width: 3}}}}},
		{name: "empty joined row", query: "abcd", rows: []text.Row{{Text: "ab"}, {Text: "", Joined: true}, {Text: "cd", Joined: true}}, want: []base.Match{{Spans: []base.Span{{Width: 2}, {Width: 0}, {Width: 2}}}}},
		{name: "matched hard newline", query: "t\ns", regex: true, rows: []text.Row{{Text: "first"}, {Text: "second"}}, want: []base.Match{{Spans: []base.Span{{Col: 4, Width: 1}, {Width: 1}}}}},
		{name: "consumed wrap spaces", query: "   a", rows: []text.Row{{Text: "before"}, {Text: "after", Joined: true, Gap: "    "}}, want: []base.Match{{Row: 1, Spans: []base.Span{{Width: 1}}}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := base.Scan(t.Context(), base.ScanConfig{Query: test.query, Regex: test.regex, Rows: test.rows})
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got.Matches, test.want) {
				t.Fatalf("matches=%+v want=%+v", got.Matches, test.want)
			}
			for _, match := range got.Matches {
				if cap(match.Spans) != len(match.Spans) {
					t.Fatal("match exposes another match's span capacity")
				}
			}
		})
	}
}

func TestSearchReturnsExplicitLimitSyntaxAndCancellationErrors(t *testing.T) {
	if _, err := base.Scan(t.Context(), base.ScanConfig{Query: "(unclosed", Regex: true}); err == nil {
		t.Fatal("invalid regex was accepted")
	}
	if _, err := base.Scan(t.Context(), base.ScanConfig{Query: "a", Rows: []text.Row{{Text: "aaa"}}, MaxMatches: 2}); !errors.Is(err, base.ErrSearchLimit) {
		t.Fatalf("dense result was silently truncated: %v", err)
	}
	if _, err := base.Scan(t.Context(), base.ScanConfig{Query: "a", MaxMatches: -1}); err == nil {
		t.Fatal("negative limit was accepted")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := base.Scan(ctx, base.ScanConfig{Query: "x"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled snapshot=%v", err)
	}
	result, err := base.Scan(t.Context(), base.ScanConfig{})
	if err != nil || result.Matches != nil {
		t.Fatalf("empty search=%+v err=%v", result, err)
	}
}
