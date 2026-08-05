package kit

import (
	"image"
	"slices"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/core/grid"
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

	// Style is an ordinary row, SelectedStyle the one under the cursor, and MatchStyle
	// the characters the query matched.
	Style, SelectedStyle, MatchStyle grid.Style
	// TitleStyle is the description beside a name.
	TitleStyle grid.Style
	// Marker is drawn in the first column of the selected row, and the same width is
	// held clear on every other row so the names stay in line.
	Marker string
	// Empty is what to say when nothing matched. Nothing at all leaves the space
	// blank, which reads as a bug rather than as an answer.
	Empty string
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
		Label{Text: p.Empty, Style: p.TitleStyle, Ellipsis: "…"}.Draw(v)
		return
	}

	marker := max(text.Width(p.Marker), 0)
	for y, found := range p.Found {
		if y >= h {
			return
		}
		style := p.Style
		if y == p.Selected {
			style = p.SelectedStyle
			if p.Marker != "" {
				v.Text(0, y, p.Marker, style)
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
			clusterStyle = style.Merge(p.MatchStyle)
		}
		v.Text(x, y, cluster, clusterStyle)
		x += text.Width(cluster)
	}

	if found.Command.Title == "" || x+2 >= w {
		return
	}
	x += 2
	Label{Text: found.Command.Title, Style: style.Merge(p.TitleStyle), Ellipsis: "…"}.
		Draw(v.Sub(image.Rect(x, y, w, y+1)))
}

// matchedIn reports whether any match offset falls in a byte range.
func matchedIn(at []int, from, to int) bool {
	return slices.ContainsFunc(at, func(offset int) bool { return offset >= from && offset < to })
}

// Dress fills the palette in from a theme and a glyph set.
func (p Palette) Dress(th Theme, g Glyphs) Palette {
	p.Style = th.Text
	p.SelectedStyle = th.Selection
	p.MatchStyle = th.Accent
	p.TitleStyle = th.Muted
	p.Marker = g.Marker + " "
	if p.Empty == "" {
		p.Empty = "no matching command"
	}
	return p
}
