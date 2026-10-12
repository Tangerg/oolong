package kit

import (
	"slices"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/layout"
	"github.com/Tangerg/oolong/core/text"
)

// Palette draws a list of commands a query matched, with the matched characters
// picked out.
//
// Picking them out is most of the value. A list of names that all look equally like
// the query tells a user nothing about why any of them is there, and with subsequence
// matching the reason is often several letters apart — showing which letters matched
// is what makes "ns" finding "new-session" obvious instead of surprising.
type Palette struct {
	// Preserve caller ranking; appearance must not rerank candidates.
	Found    []headless.Found
	Selected int

	Theme  Theme
	Glyphs Glyphs
	// Empty is what to say when nothing matched. Empty says so in words, because a
	// blank space reads as a bug rather than as an answer.
	Empty string
}

// marker is what sits in the first column of the selected row, with the same width
// held clear on every other row so the names stay in line.
func (p Palette) marker() string {
	if p.Glyphs.Marker == "" {
		return ""
	}
	return p.Glyphs.Marker + " "
}

func (p Palette) empty() string {
	if p.Empty == "" {
		return "no matching command"
	}
	return p.Empty
}

// HeightForWidth is one row per command, or one for the message when nothing matched.
func (p Palette) HeightForWidth(int) int { return max(len(p.Found), 1) }

// Draw writes the list into v, one command per row.
func (p Palette) Draw(v grid.View) {
	if v.Empty() {
		return
	}
	w, h := v.Size()
	if w <= 0 || h <= 0 {
		return
	}
	if len(p.Found) == 0 {
		Label{Text: p.empty(), Style: p.Theme.Muted, Ellipsis: p.Glyphs.Ellipsis}.Draw(v)
		return
	}

	mark := p.marker()
	marker := max(text.Width(mark), 0)
	visible := v.Visible()
	first := min(max(visible.Min.Y, 0), len(p.Found))
	last := min(max(visible.Max.Y, first), min(len(p.Found), h))
	for y := first; y < last; y++ {
		found := p.Found[y]
		style := p.Theme.Text
		if y == p.Selected {
			style = p.Theme.Selection
			if mark != "" {
				v.Text(0, y, mark, style)
			}
		}
		p.row(v, y, marker, w, found, style)
	}
}

func (p Palette) row(v grid.View, y, x, w int, found headless.Found, style grid.Style) {
	name := found.Command.Name
	// Written cluster by cluster, because the match offsets are into the bytes and a
	// cluster is what occupies a column. An offset can fall inside a cluster — a
	// pattern character can match a combining mark — so a cluster counts as matched
	// when it contains an offset rather than when it begins at one.
	for at, cluster := range text.Clusters(name) {
		if x >= w {
			return
		}
		clusterStyle := style
		if matchedIn(found.At, at, at+len(cluster)) {
			clusterStyle = style.Merge(p.Theme.Accent)
		}
		v.Text(x, y, cluster, clusterStyle)
		x = layout.Sum(x, text.Width(cluster))
	}

	if found.Command.Title == "" || layout.Sum(x, 2) >= w {
		return
	}
	x = layout.Sum(x, 2)
	Label{Text: found.Command.Title, Style: style.Merge(p.Theme.Muted), Ellipsis: p.Glyphs.Ellipsis}.
		Draw(v.Sub(grid.Area(x, y, layout.Remaining(w, x), 1)))
}

func matchedIn(at []int, from, to int) bool {
	return slices.ContainsFunc(at, func(offset int) bool { return offset >= from && offset < to })
}
