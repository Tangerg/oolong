package grid

import "unicode"

//go:generate go run gen_emoji.go

// Unicode's pinned variation-sequence data owns eligible bases. A selector after
// ordinary text or another extender cannot turn that text into a wide emoji.
// The source matches the Unicode version of go-runewidth's width tables.
// https://www.unicode.org/Public/17.0.0/ucd/emoji/emoji-variation-sequences.txt
const emojiPresentationSelector = '\ufe0f'

func hasEmojiPresentation(cluster string) bool {
	var previous rune
	for _, current := range cluster {
		if current == emojiPresentationSelector && unicode.Is(emojiPresentationBases, previous) {
			return true
		}
		previous = current
	}
	return false
}
