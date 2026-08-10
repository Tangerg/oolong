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
	_, charset, found := strings.Cut(value, ".")
	if !found {
		return false
	}
	charset = strings.ToLower(strings.ReplaceAll(charset, "-", ""))
	return strings.HasPrefix(charset, "utf8")
}
