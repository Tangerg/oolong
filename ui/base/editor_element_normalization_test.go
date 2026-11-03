package base_test

import (
	"strconv"
	"strings"
	"testing"

	"github.com/Tangerg/oolong/ui/base"
)

func TestAtomicElementNormalizationPreservesIndependentGraphemes(t *testing.T) {
	for _, test := range []struct{ name, body, want string }{
		{"empty", "", ""},
		{"combining prefix", "\u0301\u0301x", "x"},
		{"joining suffix", "x\u0605\u0605", "x"},
		{"both edges", "\u0301x\u0605", "x"},
		{"only combining", "\u0301\u0301", ""},
		{"only prepends", "\u0605\u0605", ""},
		{"ordinary combining", "cafe\u0301", "cafe\u0301"},
		{"prepended visible cluster", "\u0605x", "\u0605x"},
		{"emoji joiner", "👨‍👩‍👧‍👦", "👨‍👩‍👧‍👦"},
		{"leading joiner", "\u200d👩", "👩"},
		{"regional indicators", "🇯🇵🇨", "🇯🇵🇨"},
		{"whitespace", " \t x ", " \t x "},
		{"line endings", "a\r\nb", "a b"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var editor base.Editor
			element := editor.InsertElement(fileChip, test.body)
			if element.Text() != test.want {
				t.Fatalf("normalized %q to %q; want %q", test.body, element.Text(), test.want)
			}
			if test.want == "" {
				if !editor.Empty() || element.ID != (base.ElementID{}) {
					t.Fatalf("empty label created an element: %+v, text=%q", element, editor.Text())
				}
				return
			}
			if editor.Text() != test.want+" " || !onClusterBoundary(editor.Text(), element.Start) || !onClusterBoundary(editor.Text(), element.End) {
				t.Fatalf("element absorbs neighboring text: %+v in %q", element, editor.Text())
			}
		})
	}
}

func BenchmarkAtomicElementLeadingCombiningRun(b *testing.B) {
	for _, length := range []int{1024, 4096, 16384} {
		b.Run(strconv.Itoa(length), func(b *testing.B) {
			body := strings.Repeat("\u0301", length) + "x"
			b.ReportAllocs()
			b.SetBytes(int64(len(body)))
			b.ResetTimer()
			for range b.N {
				editor := base.NewEditor(base.EditorConfig{})
				if element := editor.InsertElement(fileChip, body); element.Text() != "x" {
					b.Fatalf("element=%q", element.Text())
				}
			}
		})
	}
}
