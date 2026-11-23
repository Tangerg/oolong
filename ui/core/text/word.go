package text

import (
	"unicode"
	"unicode/utf8"

	"github.com/Tangerg/oolong/ui/core/grid"
)

type Class uint8

const (
	Space Class = iota
	// Word is a letter, a digit or an underscore — the run a double-click takes in
	// text written in an alphabet.
	Word
	// Han stays separate from adjacent alphabetic text: script runs provide a
	// selection boundary where spaces do not.
	Han
	// Kana is Japanese hiragana and katakana, which are one boundary between them
	// because a word switches from one to the other inside itself.
	Kana
	Hangul
	// Punct selects one cluster to avoid claiming adjacent symbols.
	Punct
)

// InWord reports a class whose clusters belong to words rather than separate them.
func (c Class) InWord() bool { return c != Space && c != Punct }

func ClassOf(cluster string) Class {
	if cluster == "" {
		return Space
	}
	// A cluster is classified by its base rune; marks joined to it do not change
	// what kind of word it belongs to.
	r, _ := utf8.DecodeRuneInString(cluster)
	// Scripts are tested before letters: Han, Kana and Hangul are letters too,
	// but each forms its own run.
	switch {
	case unicode.IsSpace(r):
		return Space
	case unicode.Is(unicode.Han, r):
		return Han
	case unicode.Is(unicode.Hiragana, r) || unicode.Is(unicode.Katakana, r):
		return Kana
	case unicode.Is(unicode.Hangul, r):
		return Hangul
	case r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r):
		return Word
	default:
		return Punct
	}
}

// WordAt is the byte range of the word containing an offset, and whether there is one.
//
// A word is the run of clusters sharing the class of the one under the offset — see
// [Class]. Whitespace is not a word and reports false, so a double-click in the margin
// selects nothing rather than selecting the gap. Punctuation is its own word of one
// cluster, because a run of it is not something anybody means to have selected.
//
// The offset is pulled to a cluster boundary first, so a caller working in columns
// cannot land inside a character and get half of it.
func WordAt(s string, at int) (start, end int, ok bool) {
	return Of(s, grid.Style{}).WordAt(at)
}
