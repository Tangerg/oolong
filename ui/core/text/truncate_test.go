package text_test

import (
	"strings"
	"testing"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/text"
)

func TestTruncateKeepsAJoiningSuffixIndependent(t *testing.T) {
	for _, tc := range []struct{ source, suffix string }{
		{"❤x", "\ufe0f"},
		{"1x", "\ufe0f\u20e3"},
		{"ex", "\u0301"},
	} {
		t.Run(tc.source, func(t *testing.T) {
			got := text.Truncate(tc.source, 1, tc.suffix)
			if got != tc.suffix || text.Width(got) != 0 {
				t.Fatalf("Truncate(%q, 1, %q) = %q at width %d, want the independent zero-width suffix", tc.source, tc.suffix, got, text.Width(got))
			}
		})
	}
	if got := text.Truncate("❤", 1, "\ufe0f"); got != "❤" {
		t.Fatalf("a line that already fits changed to %q", got)
	}
}

func TestWithSuffixPreservesGraphemeAndColumnBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name, source, suffix, want string
		width                      int
	}{
		{"uncut source", "a", "…", "a…", 3},
		{"empty source", "", "...", "..", 2},
		{"oversized suffix cluster", "abc", "中", "a", 1},
		{"odd regional indicator", "x🇦", "🇧🇨", "x🇧🇨", 10},
		{"even regional indicators", "x🇦🇧", "🇨", "x🇦🇧🇨", 10},
		{"regional indicator run", "x🇦🇧🇨", "🇩🇪", "x🇦🇧🇩🇪", 10},
		{"later independent edge", "🇦x", "🇧", "🇦x🇧", 10},
		{"emoji joiner context", "x👩\u0301\u200d", "💻", "x💻", 10},
		{"Indic linker context", "xक्\u200d", "क", "xक", 10},
		{"prepend", "x\u0600", "y", "xy", 10},
		{"Hangul", "xᄀ", "ᅡ", "xᅡ", 10},
		{"removed control changes regional pairing", "🇦\x00🇧🇨", "🇩", "🇦🇧🇩", 10},
		{"removed control protects a modifier", "❤\n\ufe0fX", "!", "❤X!", 3},
		{"drawable suffix", "abcdef", "!\x1b\x00\xff", "ab!", 3},
		{"source tab", "a\tb", "…", "a  …", 4},
		{"suffix tab cannot partly fit", "0", "\t", "0", 7},
		{"suffix tab", "abcdefghijklmnopqrst", "X\t", "abcdefX\t", 15},
		{"multiple suffix tabs", "abcdefghij", "x\ty\tz", "abcdefx\ty\tz", 18},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := text.Of(tc.source, grid.Style{}).WithSuffix(tc.width, tc.suffix)
			if got.String() != tc.want || got.Width() > tc.width {
				t.Fatalf("WithSuffix(%q, %d, %q) = %q at width %d, want %q", tc.source, tc.width, tc.suffix, got.String(), got.Width(), tc.want)
			}
			if plain := text.Truncate(tc.source, tc.width, tc.suffix); text.Width(tc.source) > tc.width && plain != tc.want {
				t.Fatalf("Truncate = %q, want the same bounded composition %q", plain, tc.want)
			}
		})
	}
	for _, width := range []int{-1, 0} {
		if got := text.Of("source", grid.Style{}).WithSuffix(width, "…"); got != nil {
			t.Fatalf("WithSuffix(%d) = %v, want nil", width, got)
		}
	}
}

func TestSuffixInheritsOnlyTheRetainedStyle(t *testing.T) {
	const target = "https://example.test/visible"
	style := grid.Style{Attr: grid.Bold}
	source := text.Line{
		{Text: "x", Style: style, Link: target},
		{Text: "🇦long", Style: grid.Style{Attr: grid.Italic}, Link: "https://example.test/removed"},
	}
	for _, got := range []text.Line{source.WithSuffix(4, "🇧🇨"), source.Truncate(4, "🇧🇨")} {
		if got.String() != "x🇧🇨" {
			t.Fatalf("suffix joined retained text: %q", got.String())
		}
		surface := grid.NewSurface(4, 1)
		got.Draw(surface.View(), 0, 0)
		if cell := cellAt(surface, 0); cell.Link != target || cell.Style != style {
			t.Fatalf("retained cell = %+v, want its original style and link", cell)
		}
		for column := 1; column < got.Width(); column++ {
			if cell := cellAt(surface, column); cell.Link != "" || cell.Style != style {
				t.Fatalf("suffix cell %d = %+v, want the retained style without a link", column, cell)
			}
		}
	}
}

func FuzzWithSuffixFitsAtAnIndependentBoundary(f *testing.F) {
	for _, seed := range []struct {
		source, suffix string
		width          int
	}{
		{"❤x", "\ufe0f", 1},
		{"1x", "\ufe0f\u20e3", 1},
		{"x🇦🇧🇨", "🇩🇪", 10},
		{"x👩\u0301\u200d", "💻", 10},
		{"xक्\u200d", "क", 10},
		{"🇦\x00🇧🇨", "🇩", 10},
		{"a\tb", "x\ty\tz", 18},
		{"0", "\t", 7},
		{"A\xe2\u0301B", "!\xff", 3},
	} {
		f.Add(seed.source, seed.suffix, seed.width)
	}
	f.Fuzz(func(t *testing.T, source, suffix string, width int) {
		if len(source) > 256 || len(suffix) > 64 {
			return
		}
		width = min(max(width, 0), 64)
		got := text.Of(source, grid.Style{}).WithSuffix(width, suffix).String()
		if columns := text.Width(got); columns > width {
			t.Fatalf("WithSuffix(%q, %d, %q) = %q at width %d", source, width, suffix, got, columns)
		}
		ending := (text.Line{}).WithSuffix(width, suffix).String()
		if !strings.HasSuffix(got, ending) {
			t.Fatalf("WithSuffix(%q, %d, %q) = %q, lost suffix %q", source, width, suffix, got, ending)
		}
		if at := len(got) - len(ending); !text.IsClusterBoundary(got, at) {
			t.Fatalf("WithSuffix(%q, %d, %q) = %q, suffix at %d joins retained text", source, width, suffix, got, at)
		}
	})
}
