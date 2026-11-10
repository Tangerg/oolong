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
		{name: "occurrences", query: "cat", rows: []text.Row{{Content: text.Line{{Text: "the cat sat"}}}, {Content: text.Line{{Text: "on the mat"}}}, {Content: text.Line{{Text: "with a cat"}}}}, want: []base.Match{{Row: 0, Spans: []base.Span{{Col: 4, Width: 3}}}, {Row: 2, Spans: []base.Span{{Col: 7, Width: 3}}}}},
		{name: "case folding", query: "cat", rows: []text.Row{{Content: text.Line{{Text: "The Cat SAT"}}}}, want: []base.Match{{Spans: []base.Span{{Col: 4, Width: 3}}}}},
		{name: "folding preserves UTF-8 offsets", query: "cat", rows: []text.Row{{Content: text.Line{{Text: "İstanbul then cat"}}}}, want: []base.Match{{Spans: []base.Span{{Col: 14, Width: 3}}}}},
		{name: "wide character", query: "cat", rows: []text.Row{{Content: text.Line{{Text: "中文 cat"}}}}, want: []base.Match{{Spans: []base.Span{{Col: 5, Width: 3}}}}},
		{name: "indent", query: "me", rows: []text.Row{{Content: text.Line{{Text: "find me"}}, Offset: 3}}, want: []base.Match{{Spans: []base.Span{{Col: 8, Width: 2}}}}},
		{name: "soft wrap", query: "quick brown", rows: []text.Row{{Content: text.Line{{Text: "the quick"}}}, {Content: text.Line{{Text: "brown fox"}}, Joined: true, Gap: " "}}, want: []base.Match{{Spans: []base.Span{{Col: 4, Width: 5}, {Col: 0, Width: 5}}}}},
		{name: "hard break", query: "sat on", rows: []text.Row{{Content: text.Line{{Text: "the cat sat"}}}, {Content: text.Line{{Text: "on the mat"}}}}},
		{name: "regex", query: `error \d+`, regex: true, rows: []text.Row{{Content: text.Line{{Text: "error 42 and error 7"}}}}, want: []base.Match{{Spans: []base.Span{{Col: 0, Width: 8}}}, {Spans: []base.Span{{Col: 13, Width: 7}}}}},
		{name: "no match", query: "absent", rows: []text.Row{{Content: text.Line{{Text: "one"}}}, {Content: text.Line{{Text: "two"}}}}},
		{name: "literal", query: "a+b", rows: []text.Row{{Content: text.Line{{Text: "a+b and aab"}}}}, want: []base.Match{{Spans: []base.Span{{Width: 3}}}}},
		{name: "empty joined row", query: "abcd", rows: []text.Row{{Content: text.Line{{Text: "ab"}}}, {Content: text.Line{{Text: ""}}, Joined: true}, {Content: text.Line{{Text: "cd"}}, Joined: true}}, want: []base.Match{{Spans: []base.Span{{Width: 2}, {Width: 0}, {Width: 2}}}}},
		{name: "matched hard newline", query: "t\ns", regex: true, rows: []text.Row{{Content: text.Line{{Text: "first"}}}, {Content: text.Line{{Text: "second"}}}}, want: []base.Match{{Spans: []base.Span{{Col: 4, Width: 1}, {Width: 1}}}}},
		{name: "combining mark", query: "\u0301", rows: []text.Row{{Content: text.Line{{Text: "x e\u0301"}}}}, want: []base.Match{{Spans: []base.Span{{Col: 2, Width: 1}}}}},
		{name: "emoji component", query: "💻", rows: []text.Row{{Content: text.Line{{Text: "👩‍💻"}}}}, want: []base.Match{{Spans: []base.Span{{Width: 2}}}}},
		{name: "repeated runes inside one cell", query: ".", regex: true, rows: []text.Row{{Content: text.Line{{Text: "e\u0301"}}, Offset: 2}}, want: []base.Match{{Spans: []base.Span{{Col: 2, Width: 1}}}, {Spans: []base.Span{{Col: 2, Width: 1}}}}},
		{name: "tab in decorated row", query: "猫", rows: []text.Row{{Content: text.Line{{Text: "猫\t猫"}}, Offset: 3}}, want: []base.Match{{Spans: []base.Span{{Col: 3, Width: 2}}}, {Spans: []base.Span{{Col: 11, Width: 2}}}}},
		{name: "independent modifier", query: "🏻", rows: []text.Row{{Content: text.Line{{Text: "   "}, {Text: "🏻X", BreakBefore: true}}}}, want: []base.Match{{Spans: []base.Span{{Col: 3, Width: 2}}}}},
		{name: "following independent modifier", query: "X", rows: []text.Row{{Content: text.Line{{Text: "   "}, {Text: "🏻X", BreakBefore: true}}}}, want: []base.Match{{Spans: []base.Span{{Col: 5, Width: 1}}}}},
		{name: "filtered control is not searchable", query: "\n", rows: []text.Row{{Content: text.Line{{Text: "🇦\n🇧X"}}}}},
		{name: "display cluster crosses filtered control", query: "🇦🇧", rows: []text.Row{{Content: text.Line{{Text: "🇦\n🇧X"}}}}, want: []base.Match{{Spans: []base.Span{{Width: 2}}}}},
		{name: "byte offsets after filtered control", query: "X", rows: []text.Row{{Content: text.Line{{Text: "\x1b🇦\n🇧X"}}}}, want: []base.Match{{Spans: []base.Span{{Col: 2, Width: 1}}}}},
		{name: "malformed byte is not a column", query: "猫", rows: []text.Row{{Content: text.Line{{Text: "a\xff猫"}}}}, want: []base.Match{{Spans: []base.Span{{Col: 1, Width: 2}}}}},
		{name: "filtered gap", query: "ab", rows: []text.Row{{Content: text.Line{{Text: "a"}}}, {Content: text.Line{{Text: "b"}}, Joined: true, Gap: "\x1b"}}, want: []base.Match{{Spans: []base.Span{{Width: 1}, {Width: 1}}}}},
		{name: "line end empty match", query: "$", regex: true, rows: []text.Row{{Content: text.Line{{Text: "abc"}}, Offset: 2}}, want: []base.Match{{Spans: []base.Span{{Col: 5}}}}},
		{name: "empty displayed line", query: "^$", regex: true, rows: []text.Row{{Content: text.Line{{Text: "\x1b"}}, Offset: 2}}, want: []base.Match{{Spans: []base.Span{{Col: 2}}}}},
		{name: "entire consumed gap", query: " ", rows: []text.Row{{Content: text.Line{{Text: "before"}}}, {Content: text.Line{{Text: "after"}}, Joined: true, Gap: " ", Offset: 2}}, want: []base.Match{{Row: 1, Spans: []base.Span{{Col: 2}}}}},
		{name: "consumed wrap spaces", query: "   a", rows: []text.Row{{Content: text.Line{{Text: "before"}}}, {Content: text.Line{{Text: "after"}}, Joined: true, Gap: "    "}}, want: []base.Match{{Row: 1, Spans: []base.Span{{Width: 1}}}}},
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
	if _, err := base.Scan(t.Context(), base.ScanConfig{Query: "a", Rows: []text.Row{{Content: text.Line{{Text: "aaa"}}}}, MaxMatches: 2}); !errors.Is(err, base.ErrSearchLimit) {
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
