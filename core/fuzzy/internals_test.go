package fuzzy

// Tests of what this package keeps to itself.
//
// The word-boundary rule is what the score is built on and has no public name of
// its own, so it is asserted where it lives.
// Everything else about this package is asserted from outside it, which is where
// a caller stands.

import (
	"testing"
)

func TestSeparatorsAreWhatStartsAWordAndNotCase(t *testing.T) {
	// Treating a capital as a word start would score "SQL" as three words, and the
	// bonus is meant to find the beginning of something a person would name.
	for _, prev := range []rune{' ', '\t', '_', '-', '.', '/', ':'} {
		if !opensWord(prev) {
			t.Errorf("%q does not open a word", prev)
		}
	}
	for _, prev := range []rune{'a', 'Z', '0', '(', '\''} {
		if opensWord(prev) {
			t.Errorf("%q opens a word", prev)
		}
	}
}
