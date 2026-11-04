package text_test

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/text"
)

func TestPaddedFieldsKeepIndependentGraphemes(t *testing.T) {
	for _, test := range []struct {
		name, value string
		width       int
		alignment   layout.Align
		want        string
	}{
		{"start", "abc", 6, layout.Start, "abc   "},
		{"center", "abc", 6, layout.Center, " abc  "},
		{"end", "abc", 6, layout.End, "   abc"},
		{"wide", "界", 5, layout.Center, " 界  "},
		{"overfull", "界", 1, layout.Start, "界"},
		{"empty", "", 3, layout.End, "   "},
		{"negative", "abc", -1, layout.Center, "abc"},
		{"tabs", "a\tb", 10, layout.End, " a       b"},
		{"leading combining", "\u0301", 0, layout.Start, " \u0301"},
		{"leading modifier", "🏻", 4, layout.End, "   🏻"},
		{"trailing prepend", "\u0600", 0, layout.Start, "\u0600 "},
		{"joining padding", "\u0600\u0600", 4, layout.Center, " \u0600\u0600  "},
		{"complete grapheme", "👩‍💻", 2, layout.End, "👩‍💻"},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := text.Of(test.value, grid.Style{}).Pad(test.width, test.alignment)
			if got.String() != test.want {
				t.Fatalf("Pad = %q, want %q", got.String(), test.want)
			}
			if got.Width() != text.Width(test.want) || got.Width() < test.width {
				t.Fatalf("Pad width = %d, minimum %d", got.Width(), test.width)
			}
		})
	}
}

func TestPaddedFieldsKeepAppearanceWithTheirContent(t *testing.T) {
	const target = "https://example.com/field"
	style := grid.Style{Attr: grid.Bold}
	for _, value := range []string{"\u0301", "🏻", "\u0600", "\u0600\u0600"} {
		t.Run(value, func(t *testing.T) {
			source := text.Line{{Text: value, Style: style, Link: target}}
			field := source.Pad(6, layout.Center)
			if source.String() != value {
				t.Fatal("padding changed the source")
			}
			found := false
			for _, span := range field {
				if strings.Contains(span.Text, value) {
					found = true
					if span.Style != style || span.Link != target {
						t.Fatalf("field lost its appearance: %+v", span)
					}
				} else if span.Style != (grid.Style{}) || span.Link != "" {
					t.Fatalf("padding took content appearance: %+v", span)
				}
			}
			if !found {
				t.Fatalf("field lost its content: %q", field.String())
			}
		})
	}
}

func FuzzPaddedFieldsKeepTheirColumnBoundaries(f *testing.F) {
	for _, value := range []string{"", "abc", "界", "a\tb", "\u0301", "🏻", "\u0600", "\u0600\u0600", "ाा", "🇺🇸ा🇨", "👩‍💻"} {
		f.Add(value, uint8(9), uint8(layout.Center))
	}
	f.Fuzz(func(t *testing.T, value string, extent, align uint8) {
		if len(value) > 256 {
			return
		}
		width := int(extent) % 64
		source := text.Of(value, grid.Style{})
		natural := source.Pad(0, layout.Start)
		field := source.Pad(width, layout.Align(align%3))
		content := field.String()
		if got, want := field.Width(), max(width, natural.Width()); got != want {
			t.Fatalf("Pad(%q, %d) uses %d columns, want %d: %q", value, width, got, want, content)
		}
		if !utf8.ValidString(content) || text.Printable(content) != content || strings.ContainsRune(content, '\t') {
			t.Fatalf("Pad produced non-normalized text %q", content)
		}
		if text.NextCluster(" "+content, 0) > 1 || text.PrevCluster(content+" ", len(content)+1) < len(content) {
			t.Fatalf("field still joins its neighbours: %q", content)
		}
		if again := field.Pad(width, layout.Start); again.String() != content {
			t.Fatalf("field preparation is not stable: %q became %q", content, again.String())
		}
	})
}
