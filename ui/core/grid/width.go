package grid

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Tangerg/oolong/ui/core/internal/grapheme"
	"github.com/Tangerg/oolong/ui/core/layout"
)

func graphemeWidth(s string) int {
	total := 0
	clusters := grapheme.New(s)
	for clusters.Next() {
		total = layout.Sum(total, ClusterWidth(clusters.Value()))
	}
	return total
}

// ClusterWidth is shared by measurement and drawing so terminal-column
// geometry has one authority. Width remains an estimate of terminal behavior:
// ambiguous characters use one column, an emoji presentation sequence uses two,
// and halfwidth-katakana sound marks U+FF9E and U+FF9F contribute their own
// spacing columns. Controls measure zero.
func ClusterWidth(cluster string) int {
	if control(cluster) {
		return 0
	}
	marks := 0
	// The dependency caps a cluster at two columns. Count spacing marks as they
	// leave the base so the same classification owns their additional columns.
	base := strings.Map(func(r rune) rune {
		if r == halfwidthKatakanaVoicedSoundMark || r == halfwidthKatakanaSemivoicedSoundMark {
			marks++
			return -1
		}
		return r
	}, cluster)
	width := max(columns.StringWidth(base), 0)
	// Test the original adjacency: removing a spacing mark cannot turn a
	// selector after that mark into a presentation request for an earlier base.
	if hasEmojiPresentation(cluster) {
		width = max(width, 2)
	}
	return layout.Sum(width, marks)
}

// Reject control bytes before they enter a cell: repaint writes cell content
// verbatim, so a stored Escape or tab would become an instruction instead of text.
func control(cluster string) bool {
	if cluster == "" {
		return false
	}
	if !utf8.ValidString(cluster) {
		return true
	}
	r, _ := utf8.DecodeRuneInString(cluster)
	return unicode.IsControl(r)
}
