package text

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// Printable returns the terminal-safe text in s.
//
// A tab is kept, because it is laid out rather than obeyed — see [TabStop] — and
// every other control character is an instruction for a terminal this package is
// not asking it to perform. Invalid UTF-8 becomes replacement text. Cleaning occurs
// before layout rather than at the cell so measuring, cursor offsets and drawing see
// the same text.
//
// A newline is a control character too. Call Printable on one logical line at a time
// when line breaks carry meaning of their own.
func Printable(s string) string {
	if utf8.ValidString(s) && !strings.ContainsFunc(s, dropRune) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range strings.ToValidUTF8(s, "\ufffd") {
		if !dropRune(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func dropRune(r rune) bool { return r != '\t' && unicode.IsControl(r) }
