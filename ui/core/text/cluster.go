package text

import (
	"iter"
	"unicode"
	"unicode/utf8"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/internal/grapheme"
	"github.com/Tangerg/oolong/ui/core/layout"
)

// Width is how many columns s would occupy, with tabs expanded from column zero.
func Width(s string) int { return Advance(s, 0) }

// Advance returns the final terminal column after drawing s from col. Tabs use
// their position in the whole line even when a caller measures it in pieces
// separated at display grapheme boundaries after filtering. Negative starting
// columns count as zero.
func Advance(s string, col int) int {
	col = max(0, col)
	if plainASCII(s) {
		return layout.Sum(col, len(s))
	}
	for cluster := range projectDisplay(s).clusters() {
		col = advanceCluster(cluster.Text, col)
	}
	return col
}

func advanceCluster(cluster string, col int) int {
	if cluster == "\t" {
		return layout.Sum(col, TabStop-col%TabStop)
	}
	return layout.Sum(col, grid.ClusterWidth(cluster))
}

// plainASCII reports whether every byte is one printable, one-column cluster.
// Keeping that fact here lets every byte/column conversion share the common fast
// path without weakening the Unicode path used as the authority for all other text.
func plainASCII(s string) bool {
	for i := range len(s) {
		if !plainASCIIByte(s[i]) {
			return false
		}
	}
	return true
}

func plainASCIIByte(b byte) bool { return b >= ' ' && b <= '~' }

// Measurement and drawing share dropped so their widths agree. Unlike
// Printable, display projection omits malformed bytes; stored text retains a
// replacement marker for them.
func dropped(cluster string) bool {
	if !utf8.ValidString(cluster) {
		return true
	}
	r, _ := utf8.DecodeRuneInString(cluster)
	return droppedRune(r)
}

// Tabs are expanded by layout; other controls must not reach terminal output.
func droppedRune(r rune) bool { return r != '\t' && unicode.IsControl(r) }

// Clusters preserves grapheme boundaries because a cursor between a base and
// its combining mark has no display position. Malformed bytes remain separate
// zero-width clusters so later offsets still address the original input.
func Clusters(s string) iter.Seq2[int, string] {
	return func(yield func(int, string) bool) {
		clusters := grapheme.New(s)
		for clusters.Next() {
			if !yield(clusters.Start(), clusters.Value()) {
				return
			}
		}
	}
}

// IsClusterBoundary tests a byte offset in the complete string. Concatenation can
// move boundaries without changing the total cluster count, as regional indicators
// pair with different neighbors. Out-of-range offsets are not boundaries.
func IsClusterBoundary(s string, at int) bool {
	return at >= 0 && at <= len(s) && (at == 0 || NextCluster(s, at-1) == at)
}

// NextCluster is the byte offset after the grapheme cluster at i, or len(s) at the
// end. Like [Clusters] and [PrevCluster] it segments the source text as written; a
// [Line] steps display clusters instead, after dropping controls that draw nothing,
// so the two agree on text [Printable] has already cleaned.
func NextCluster(s string, i int) int {
	if i < 0 {
		return 0
	}
	if i >= len(s) {
		return len(s)
	}
	// Plain ASCII is one cluster per byte. Requiring ASCII on both sides keeps this
	// fast path out of the cases where adjacent Unicode can join it (Prepend, Extend
	// and ZWJ sequences). CRLF is the one multi-byte ASCII cluster.
	if s[i] < utf8.RuneSelf && (i == 0 || s[i-1] < utf8.RuneSelf) &&
		(i+1 == len(s) || s[i+1] < utf8.RuneSelf) {
		if s[i] == '\r' && i+1 < len(s) && s[i+1] == '\n' {
			return i + 2
		}
		return i + 1
	}
	for at, cluster := range Clusters(s) {
		if after := at + len(cluster); i < after {
			return after
		}
	}
	return len(s)
}

// PrevCluster is the byte offset of the grapheme cluster ending at i, or zero at the
// start. It segments the source text as written, as [NextCluster] does.
func PrevCluster(s string, i int) int {
	if i <= 0 {
		return 0
	}
	i = min(i, len(s))
	if i == 0 {
		return 0
	}
	last := i - 1
	if s[last] < utf8.RuneSelf && (last == 0 || s[last-1] < utf8.RuneSelf) {
		if s[last] == '\n' && last > 0 && s[last-1] == '\r' {
			return last - 1
		}
		return last
	}
	for at, cluster := range Clusters(s) {
		if i <= at+len(cluster) {
			return at
		}
	}
	return len(s)
}

// ColumnOf maps original bytes onto the filtered display. An offset inside a
// display grapheme maps after the whole atom, even when filtering joined it.
func ColumnOf(s string, i int) int {
	i = min(max(i, 0), len(s))
	// Only the prefix before i contributes to the answer. The byte at i need only be
	// ASCII, not printable: a control there occupies no preceding column, while a
	// Unicode Extend or ZWJ may still join the cluster before the boundary.
	if plainASCII(s[:i]) && (i == len(s) || s[i] < utf8.RuneSelf) {
		return i
	}
	return Of(s, grid.Style{}).ColumnOf(i)
}

// OffsetAt is the original byte offset of the display boundary nearest to col,
// without going past it. It is how a click, or a cursor moving between lines of
// different lengths, finds where it lands.
func OffsetAt(s string, col int) int {
	if col <= 0 {
		return 0
	}
	end := min(col, len(s))
	// Unlike ColumnOf, the byte at the candidate boundary must itself occupy one
	// column. A tab or zero-width control belongs to this column and must be consumed;
	// a Unicode byte may join or widen the preceding cluster.
	if plainASCII(s[:end]) && (end == len(s) || plainASCIIByte(s[end])) {
		return end
	}
	return Of(s, grid.Style{}).OffsetAt(col)
}
