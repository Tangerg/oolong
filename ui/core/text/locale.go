package text

import "strings"

// PrefersUnicode reports whether a POSIX locale should select Unicode terminal
// text. An empty locale keeps the modern Unicode default; C, POSIX, a language
// without an encoding, and a locale naming another encoding return false.
//
// Resolving which locale belongs to a terminal is a transport concern. Interpreting
// how that locale constrains text belongs here, beside the text that must be
// representable in it.
func PrefersUnicode(value string) bool {
	if value == "" {
		return true
	}
	base, _, _ := strings.Cut(value, "@")
	_, charset, found := strings.Cut(base, ".")
	if !found {
		return false
	}
	return strings.EqualFold(charset, "utf8") || strings.EqualFold(charset, "utf-8")
}
