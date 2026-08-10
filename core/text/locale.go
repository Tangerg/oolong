package text

import "strings"

// UTF8Locale reports whether a POSIX locale explicitly names the UTF-8 character
// set. It does not guess: C, POSIX, an empty locale and a language without an
// encoding all return false.
//
// Resolving which locale belongs to a terminal is a transport concern. Interpreting
// the encoding it names belongs here, beside the text that must be representable in
// that encoding.
func UTF8Locale(value string) bool {
	_, charset, found := strings.Cut(value, ".")
	if !found {
		return false
	}
	charset = strings.ToLower(strings.ReplaceAll(charset, "-", ""))
	return strings.HasPrefix(charset, "utf8")
}
