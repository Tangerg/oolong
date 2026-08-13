package text_test

import (
	"strings"
	"testing"

	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/text"
)

var benchmarkCoordinate int

func BenchmarkWrap(b *testing.B) {
	// What a transcript pays every time it is laid out at a new width.
	line := text.Of(strings.Repeat("the quick brown fox jumps over the lazy dog ", 20), grid.Style{})

	b.ReportAllocs()
	for b.Loop() {
		_ = line.Wrap(80)
	}
}

func BenchmarkWidth(b *testing.B) {
	// The one width authority, which everything that measures or draws goes
	// through.
	const s = "the quick brown fox 中文 with a combining é and an emoji 🙂"

	b.ReportAllocs()
	for b.Loop() {
		_ = text.Width(s)
	}
}

func BenchmarkColumnOf(b *testing.B) {
	for _, tc := range []struct {
		name   string
		text   string
		offset int
	}{
		{"plain/end", strings.Repeat("the quick brown fox ", 8), -1},
		{"plain/early-in-long", strings.Repeat("a", 10_000), 10},
		{"unicode/end", strings.Repeat("the quick 中文 fox ", 8), -1},
		{"unicode/after-early-offset", strings.Repeat("a", 10_000) + "中", 10},
	} {
		b.Run(tc.name, func(b *testing.B) {
			offset := tc.offset
			if offset < 0 {
				offset = len(tc.text)
			}
			b.ReportAllocs()
			for b.Loop() {
				benchmarkCoordinate = text.ColumnOf(tc.text, offset)
			}
		})
	}
}

func BenchmarkOffsetAt(b *testing.B) {
	for _, tc := range []struct {
		name   string
		text   string
		column int
	}{
		{"plain/end", strings.Repeat("the quick brown fox ", 8), -1},
		{"plain/early-in-long", strings.Repeat("a", 10_000), 10},
		{"unicode/end", strings.Repeat("the quick 中文 fox ", 8), -1},
		{"unicode/after-early-column", strings.Repeat("a", 10_000) + "中", 10},
	} {
		b.Run(tc.name, func(b *testing.B) {
			column := tc.column
			if column < 0 {
				column = len(tc.text)
			}
			b.ReportAllocs()
			for b.Loop() {
				benchmarkCoordinate = text.OffsetAt(tc.text, column)
			}
		})
	}
}

func BenchmarkTruncate(b *testing.B) {
	const s = "the quick brown fox jumps over the lazy dog and keeps going for a while"

	b.ReportAllocs()
	for b.Loop() {
		_ = text.Truncate(s, 40, "…")
	}
}
