package text

import (
	"strings"
	"unicode/utf8"
)

// Printable cleans one logical line before layout so measurement, byte offsets,
// and drawing agree. Tabs remain for layout; other controls, including newlines,
// are removed. Each malformed UTF-8 byte becomes a replacement rune.
func Printable(s string) string {
	if utf8.ValidString(s) && !strings.ContainsFunc(s, droppedRune) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if !droppedRune(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}
