package grapheme

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/clipperhouse/uax29/v2/graphemes"
)

func TestValidationWindowsPreserveUnicodeBoundaries(t *testing.T) {
	for _, sequence := range []string{
		"\r\n",
		"a\u0301",
		"👩‍💻",
		"🇨🇳🇬",
		"🇨🇳🇬🇧",
		"\u0600a",
		"\u0915\u094d\u0915",
		"a" + strings.Repeat("\u0301", initialValidationBytes*initialValidationBytes),
	} {
		for padding := 0; padding <= initialValidationBytes+utf8.UTFMax; padding++ {
			source := strings.Repeat("x", padding) + sequence + strings.Repeat("y", initialValidationBytes) + sequence + "z"
			assertUnicodeSegmentation(t, source)
		}
	}
}

func TestMalformedBytesCloseValidationWindows(t *testing.T) {
	for padding := 0; padding <= initialValidationBytes+utf8.UTFMax; padding++ {
		prefix := strings.Repeat("x", padding) + "👩‍💻"
		source := prefix + "\xe2\u0301\ufffdz"
		iterator := New(source)
		for iterator.Next() && iterator.Start() < len(prefix) {
			if !utf8.ValidString(iterator.Value()) {
				t.Fatalf("padding %d: valid prefix absorbed corruption: %q", padding, iterator.Value())
			}
		}
		offset := len(prefix)
		for _, want := range []string{"\xe2", "\u0301", "\ufffd", "z"} {
			if iterator.Start() != offset || iterator.Value() != want {
				t.Fatalf("padding %d: cluster = %q at %d, want %q at %d", padding, iterator.Value(), iterator.Start(), want, offset)
			}
			offset += len(want)
			if more := iterator.Next(); more != (offset < len(source)) {
				t.Fatalf("padding %d: Next at %d = %t", padding, offset, more)
			}
		}
	}
}

func FuzzValidUTF8MatchesUnicodeSegmentation(f *testing.F) {
	for _, source := range []string{
		"", "plain ASCII", "中👩‍💻ｶﾞ", "\r\n", "🇨🇳🇬🇧", "\u0600a", "\u0915\u094d\u0915",
		strings.Repeat("x", initialValidationBytes-1) + "a" + strings.Repeat("\u0301", initialValidationBytes),
	} {
		f.Add(source)
	}
	f.Fuzz(func(t *testing.T, source string) {
		assertUnicodeSegmentation(t, strings.ToValidUTF8(source, string(utf8.RuneError)))
	})
}

func assertUnicodeSegmentation(t *testing.T, source string) {
	t.Helper()
	got := New(source)
	want := graphemes.FromString(source)
	for got.Next() {
		if !want.Next() || got.Start() != want.Start() || got.End() != want.End() || got.Value() != want.Value() {
			t.Fatalf("source %q: cluster = %q at [%d,%d), want %q at [%d,%d)",
				source, got.Value(), got.Start(), got.End(), want.Value(), want.Start(), want.End())
		}
	}
	if want.Next() {
		t.Fatalf("source %q: stopped before cluster %q at %d", source, want.Value(), want.Start())
	}
}
