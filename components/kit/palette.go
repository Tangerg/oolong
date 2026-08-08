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
	// Found is what to show, best first, as the registry ranked it.
	Found []headless.Found
	// Selected is the index under the cursor.
	Selected int

	// Theme is the look. Every part of a row has a fixed role in one — a name is
	// text, the row under the cursor is the selection, a matched letter is the accent
	// — so there is nothing here to choose between.
	Theme Theme
	// Glyphs are the characters the marker is drawn with, which is a fact about the
	// terminal rather than about the look.
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

// empty is what to say when nothing matched.
func (p Palette) empty() string {
	if p.Empty == "" {
		return "no matching command"
	}
	return p.Empty
}

// Measure is one row per command, or one for the message when nothing matched.
func (p Palette) Measure(int) int { return max(len(p.Found), 1) }

// Draw writes the list into v, one command per row.
func (p Palette) Draw(v grid.View) {
	w, h := v.Size()
	if w <= 0 || h <= 0 {
		return
	}
	if len(p.Found) == 0 {
		Label{Text: p.empty(), Style: p.Theme.Muted, Ellipsis: "…"}.Draw(v)
		return
	}

	mark := p.marker()
	marker := max(text.Width(mark), 0)
	for y, found := range p.Found {
		if y >= h {
			return
		}
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

// row draws one command: its name with the matched characters picked out, then its
// description in whatever room is left.
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
	Label{Text: found.Command.Title, Style: style.Merge(p.Theme.Muted), Ellipsis: "…"}.
		Draw(v.Sub(grid.Rect(x, y, layout.Remaining(w, x), 1)))
}

// matchedIn reports whether any match offset falls in a byte range.
func matchedIn(at []int, from, to int) bool {
	return slices.ContainsFunc(at, func(offset int) bool { return offset >= from && offset < to })
}
