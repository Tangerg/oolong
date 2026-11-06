package text

import (
	"strings"
	"unicode/utf8"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/layout"
)

// Truncate cuts the line to at most width columns, adding ellipsis only when the
// line exceeds that width. An uncut line keeps its tabs; a cut uses [Line.WithSuffix]
// so the marker cannot alter the last retained grapheme or acquire its hyperlink.
func (l Line) Truncate(width int, ellipsis string) Line {
	if width <= 0 {
		return nil
	}
	used := 0
	for u := range l.eachUnit() {
		used = layout.Sum(used, u.width)
		if used > width {
			return l.WithSuffix(width, ellipsis)
		}
	}
	return l.printable()
}

// WithSuffix appends suffix within width columns, shortening the line as needed.
// It preserves the longest drawable prefix that both fits and has a grapheme
// boundary before the suffix. This boundary keeps the suffix from modifying source
// text or inheriting its hyperlink. A joining-only suffix, such as a variation
// selector, therefore survives alone, with no source prefix.
//
// The suffix takes the last retained source atom's style, or the zero style if none
// survives, and never carries a link. Source tabs become their original spaces;
// suffix tabs advance from the retained prefix. A suffix wider than the budget is
// itself cut at a grapheme boundary. Controls and malformed bytes are omitted from
// both parts, and nonpositive widths return no text.
func (l Line) WithSuffix(width int, suffix string) Line {
	if width <= 0 {
		return nil
	}
	ending := newSuffix(suffix, width)
	var kept []unit
	used := 0
	// Removing a separator can regroup regional indicators. Boundary checks need
	// the drawable projection's clusters, not the original separated clusters.
	for u := range l.printable().eachUnit() {
		next := layout.Sum(used, u.width)
		if ending.endColumn(next) > width {
			break
		}
		kept = append(kept, u)
		used = next
	}
	// Only the final cluster carries context across its trailing edge. The next
	// rune decides that edge, so neither the prefix nor a long suffix is rescanned.
	for len(kept) > 0 && joins(kept[len(kept)-1].cluster, ending.firstRune) {
		kept = kept[:len(kept)-1]
	}
	out := line(kept)
	if ending.value == "" {
		return out
	}
	style := grid.Style{}
	if n := len(kept); n > 0 {
		style = kept[n-1].style()
	}
	if n := len(out); n > 0 && out[n-1].Style == style && out[n-1].Link == "" {
		out[n-1].Text += ending.value
		return out
	}
	return append(out, Span{Text: ending.value, Style: style})
}

// Truncate is [Line.Truncate] on unstyled text.
func Truncate(s string, width int, ellipsis string) string {
	return Of(s, grid.Style{}).Truncate(width, ellipsis).String()
}

type suffix struct {
	value, firstRune     string
	headWidth, tailWidth int
	hasTab               bool
}

func newSuffix(value string, width int) suffix {
	value = prefix(Of(value, grid.Style{}).printable().String(), width)
	_, size := utf8.DecodeRuneInString(value)
	head, tail, hasTab := strings.Cut(value, "\t")
	return suffix{
		value: value, firstRune: value[:size],
		headWidth: Width(head), tailWidth: Width(tail), hasTab: hasTab,
	}
}

func (s suffix) endColumn(column int) int {
	end := layout.Sum(column, s.headWidth)
	if s.hasTab {
		// A tab restores a column modulo TabStop, making everything after the
		// first tab independent of where the suffix began.
		end = layout.Sum(advanceCluster("\t", end), s.tailWidth)
	}
	return end
}
