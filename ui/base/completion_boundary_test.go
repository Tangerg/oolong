package base_test

import (
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/text"
)

func TestTokenAtRejectsPartialGraphemeCoordinates(t *testing.T) {
	for _, query := range []string{
		"中", "e\u0301", "👩‍💻", "🇨🇳",
		strings.Repeat("a", 31), strings.Repeat("a", 63), strings.Repeat("e\u0301", 32),
	} {
		line := "@" + query
		for cursor := -1; cursor <= len(line)+1; cursor++ {
			token, ok := base.TokenAt(line, cursor, base.Trigger{Prefix: "@"})
			want := cursor >= 1 && text.IsClusterBoundary(line, cursor)
			if ok != want {
				t.Errorf("TokenAt(%q, %d): found=%v, want %v", line, cursor, ok, want)
				continue
			}
			if ok && (!utf8.ValidString(token.Query) || token.Query != line[1:cursor] || token.Start != 1 || token.End != len(line)) {
				t.Errorf("TokenAt(%q, %d) = %+v", line, cursor, token)
			}
		}
	}
}

func TestTokenAtSkipsLongRunsOfPartialGraphemeTriggers(t *testing.T) {
	rejected := strings.Repeat("!@\u0301", 5000)
	trigger := base.Trigger{Prefix: "@"}
	if _, ok := base.TokenAt(rejected, len(rejected), trigger); ok {
		t.Fatal("partial-grapheme triggers produced a token")
	}
	line := "@" + rejected
	if token, ok := base.TokenAt(line, len(line), trigger); !ok || token.Start != 1 || token.End != len(line) || token.Query != rejected {
		t.Fatal("rejected candidates hid the earlier complete trigger")
	}
	line = rejected + " @valid"
	if token, ok := base.TokenAt(line, len(line), trigger); !ok || token.Start != len(rejected)+2 || token.End != len(line) || token.Query != "valid" {
		t.Fatal("rejected candidates replaced the later complete trigger")
	}
}

func BenchmarkTokenAtRejectedGraphemePrefixes(b *testing.B) {
	for _, count := range []int{2000, 5000} {
		line := strings.Repeat("!@\u0301", count)
		b.Run(strconv.Itoa(len(line))+"_bytes", func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(line)))
			for b.Loop() {
				base.TokenAt(line, len(line), base.Trigger{Prefix: "@"})
			}
		})
	}
}

func TestTokenAtPreservesTriggerAndReplacementBoundaries(t *testing.T) {
	for _, test := range []struct {
		name   string
		line   string
		prefix string
		query  string
		found  bool
	}{
		{name: "malformed source", line: "@\xff", prefix: "@"},
		{name: "malformed trigger matching part of a rune", line: "中query", prefix: "\xe4"},
		{name: "trigger ends within a grapheme", line: "@\u0301query", prefix: "@"},
		{name: "trigger starts within a grapheme", line: "@\u0301query", prefix: "\u0301"},
		{name: "trigger ends within joined emoji", line: "👩‍💻query", prefix: "👩"},
		{name: "trigger starts within joined emoji", line: "👩‍💻query", prefix: "💻"},
		{name: "trigger ends within a flag", line: "🇨🇳query", prefix: "🇨"},
		{name: "complete grapheme trigger", line: "@\u0301query", prefix: "@\u0301", query: "query", found: true},
		{name: "complete emoji trigger", line: "👩‍💻query", prefix: "👩‍💻", query: "query", found: true},
		{name: "complete non-ASCII trigger", line: "中query", prefix: "中", query: "query", found: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			token, ok := base.TokenAt(test.line, len(test.line), base.Trigger{Prefix: test.prefix})
			if ok != test.found || ok && (token.Query != test.query || !text.IsClusterBoundary(test.line, token.Start) || !text.IsClusterBoundary(test.line, token.End)) {
				t.Fatalf("TokenAt(%q, %q): token=%+v found=%v", test.line, test.prefix, token, ok)
			}
		})
	}
	line := "@\u0600 trailing"
	if _, ok := base.TokenAt(line, 1, base.Trigger{Prefix: "@"}); ok {
		t.Fatal("replacement would end between a prepend character and its space")
	}
	line = "@valid!@\u0301query"
	if token, ok := base.TokenAt(line, len(line), base.Trigger{Prefix: "@"}); !ok || token.Query != "valid!@\u0301query" {
		t.Fatalf("invalid later trigger suppressed an earlier complete token: %+v, %v", token, ok)
	}
}
