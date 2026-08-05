package text_test

import (
	"strings"
	"testing"

	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/text"
)

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

func BenchmarkTruncate(b *testing.B) {
	const s = "the quick brown fox jumps over the lazy dog and keeps going for a while"

	b.ReportAllocs()
	for b.Loop() {
		_ = text.Truncate(s, 40, "…")
	}
}
