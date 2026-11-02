package markdown

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/yuin/goldmark"
)

func TestNumericReferencesUseCommonMarkUnicodeCharacters(t *testing.T) {
	for _, codepoint := range []uint32{0, 0x80, 0x85, 0x91, 0x9f, 0x4e2d, 0xd800, 0xdfff, 0x10ffff, 0x110000} {
		for _, source := range []string{fmt.Sprintf("&#%d;", codepoint), fmt.Sprintf("&#x%x;", codepoint)} {
			t.Run(source, func(t *testing.T) {
				var html bytes.Buffer
				if err := goldmark.Convert([]byte(source), &html); err != nil {
					t.Fatal(err)
				}
				want := strings.TrimSuffix(strings.TrimPrefix(html.String(), "<p>"), "</p>\n")
				if got := markdownText(source); got != want {
					t.Fatalf("character reference %q = %q, CommonMark renderer produced %q", source, got, want)
				}
			})
		}
	}
}
