package text_test

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/Tangerg/oolong/ui/core/text"
)

func TestFillOwnsTerminalColumnPadding(t *testing.T) {
	for _, test := range []struct {
		name, pattern string
		width         int
		want          string
	}{
		{"negative", "x", -1, ""},
		{"zero", "x", 0, ""},
		{"empty", "", 3, "   "},
		{"zero width", "\u0301", 3, "   "},
		{"whole pattern", "ab", 5, "abab "},
		{"resegmented pattern", "🇺🇸ा🇨", 6, "🇺🇸ा🇨   "},
		{"wide", "界", 5, "界界 "},
		{"wide remainder", "界", 1, " "},
		{"tab", "a\tb", 19, "a       ba       b "},
		{"controls", "a\nb\x1b", 5, "abab "},
		{"malformed UTF-8", "a\xffb", 5, "abab "},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := text.Fill(test.pattern, test.width)
			if got != test.want {
				t.Errorf("Fill = %q, want %q", got, test.want)
			}
			if measured := text.Width(got); measured != max(0, test.width) {
				t.Errorf("Fill uses %d columns, want %d", measured, max(0, test.width))
			}
		})
	}
}

func FuzzFillUsesExactlyTheRequestedColumns(f *testing.F) {
	for _, pattern := range []string{"", "─", "界", "a\tb", "\u0301", "🇺", "\ufe0f❤", "👨\u200d", "\u0600", "A\xff\u0301B"} {
		f.Add(pattern, uint8(19))
	}
	f.Add("🇺🇸ा🇨", uint8(6))
	f.Fuzz(func(t *testing.T, pattern string, extent uint8) {
		if len(pattern) > 128 {
			return
		}
		width := int(extent) % 64
		filled := text.Fill(pattern, width)
		if got := text.Width(filled); got != width {
			t.Fatalf("pattern %q, width %d: %q occupies %d columns", pattern, width, filled, got)
		}
		if !utf8.ValidString(filled) || text.Printable(filled) != filled || strings.ContainsRune(filled, '\t') {
			t.Fatalf("pattern %q produced non-normalized output %q", pattern, filled)
		}
		if pattern != "" && text.Printable(pattern) == pattern && !strings.ContainsRune(pattern, '\t') {
			remainder := filled
			for strings.HasPrefix(remainder, pattern) {
				remainder = strings.TrimPrefix(remainder, pattern)
			}
			if strings.Trim(remainder, " ") != "" {
				t.Fatalf("pattern %q was cut inside a copy: %q", pattern, filled)
			}
		}
	})
}
