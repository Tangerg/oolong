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

var lineEndings = strings.NewReplacer("\r\n", "\n", "\r", "\n")

// NormalizeLineEndings turns CRLF and lone CR into LF. Every source format reads
// lines the same way, so a document written on any platform splits into the same
// lines everywhere it is parsed or edited.
func NormalizeLineEndings(s string) string { return lineEndings.Replace(s) }
