// Package text shares terminal-column layout across measurement and drawing so
// wrapping, carets, and cells cannot disagree. Display atoms remain indivisible,
// including wide grapheme clusters and zero-width combining marks. Decoder turns
// command output into the same styled spans after removing terminal commands.
package text

import (
	"iter"
	"slices"
	"strings"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/layout"
)

// TabStop matches conventional terminal tab stops so previously formatted
// command output retains its column relationships.
const TabStop = 8

type Span struct {
	Text  string
	Style grid.Style
	// Links travel with spans through wrapping and clipping; original byte
	// offsets alone cannot identify the resulting terminal cells.
	Link string
	// BreakBefore keeps this span's first grapheme independent of preceding text.
	// Ordinary appearance changes leave it false so they cannot split a grapheme.
	BreakBefore bool
}

// Line is one logical line of styled text — logical in that it has no width yet.
// Wrapping turns it into however many rows it needs.
type Line []Span

// Clone returns an independently owned copy of the line.
//
// It copies the span storage and detaches text and link strings from their source
// allocations. The latter matters at long-lived ownership boundaries: a short span
// sliced from a command's output must not keep the command's complete output alive.
func (l Line) Clone() Line {
	if len(l) == 0 {
		return nil
	}
	out := make(Line, len(l))
	for i, span := range l {
		span.Text = strings.Clone(span.Text)
		span.Link = strings.Clone(span.Link)
		out[i] = span
	}
	return out
}

// CloneLines returns an independently owned copy of lines and every line in it.
// It is the collection counterpart to [Line.Clone]; a caller taking ownership of a
// rendered document should not have to rebuild that deep-copy boundary itself.
func CloneLines(lines []Line) []Line {
	if len(lines) == 0 {
		return nil
	}
	out := make([]Line, len(lines))
	for i, line := range lines {
		out[i] = line.Clone()
	}
	return out
}

func Of(s string, style grid.Style) Line {
	if s == "" {
		return nil
	}
	return Line{{Text: s, Style: style}}
}

// Highlight styles each display grapheme containing a matched source byte offset.
// The original bytes remain intact so Line layout owns tab stops and clipping.
// Dropped and out-of-range byte offsets have no visible effect.
func Highlight(value string, offsets []int, normal, matched grid.Style) Line {
	if len(offsets) == 0 || normal == matched {
		return Of(value, normal)
	}
	if !slices.IsSorted(offsets) {
		offsets = slices.Clone(offsets)
		slices.Sort(offsets)
	}
	var line Line
	start := 0
	style := normal
	projection := projectDisplay(value)
	for cluster := range projection.clusters() {
		at := cluster.From
		hit, remaining := projection.match(cluster, offsets)
		offsets = remaining
		next := normal
		if hit {
			next = matched
		}
		if next != style {
			if start < at {
				line = append(line, Span{Text: value[start:at], Style: style})
			}
			start, style = at, next
		}
		if len(offsets) == 0 && style == normal {
			break
		}
	}
	if start < len(value) {
		line = append(line, Span{Text: value[start:], Style: style})
	}
	return line
}

// String serializes the content for copying. Explicit grapheme boundaries belong
// to the Line and are not encoded in the returned bytes; geometry uses Line methods.
func (l Line) String() string {
	if len(l) == 1 {
		return l[0].Text
	}
	var b strings.Builder
	for _, s := range l {
		b.WriteString(s.Text)
	}
	return b.String()
}

// Width is how many columns the line would occupy unwrapped, with tabs expanded.
func (l Line) Width() int {
	asciiWidth := 0
	ascii := true
	for _, span := range l {
		if !plainASCII(span.Text) {
			ascii = false
			break
		}
		asciiWidth = layout.Sum(asciiWidth, len(span.Text))
	}
	if ascii {
		return asciiWidth
	}
	width := 0
	for u := range l.eachUnit() {
		width = layout.Sum(width, u.width)
	}
	return width
}

// Draw writes the line onto v at (x, y) and returns how many columns it advanced.
// Tabs are expanded from the line's own start, not from the view's, so a line
// drawn at an indent keeps the column relationships it was written with.
//
// A span that points somewhere is stamped onto the columns it took, so the text a
// terminal shows is the text a terminal will open — and a link that a wrap broke in
// two is stamped on both halves, which is how one hyperlink covers two rows.
// The line owns its geometry: a standalone zero-width cluster cannot borrow a
// preceding canvas cell, including an unpainted cell skipped by a tab.
func (l Line) Draw(v grid.View, x, y int) int {
	col, linkStart := 0, 0
	var target string
	endLink := func() {
		if target != "" {
			v.Link(layout.Translate(x, linkStart), y, col-linkStart, target)
		}
	}
	for u := range l.eachUnit() {
		if link := u.link(); link != target {
			endLink()
			linkStart, target = col, link
		}
		if u.linked && u.width > 0 {
			v.Text(layout.Translate(x, col), y, u.cluster, u.style())
		}
		col = layout.Sum(col, u.width)
	}
	endLink()
	return col
}

// Tile repeats the line across row y of v, as a rule or divider does. Repetitions
// are anchored at column zero, so a view clipped on the left still shows each one
// where it falls. The last repetition is cut to the view's width rather than left to
// spill a wide cluster across its edge.
func (l Line) Tile(v grid.View, y int) {
	visible := v.Visible()
	step := l.Width()
	if step <= 0 || y < visible.Min.Y || y >= visible.Max.Y {
		return
	}
	width, _ := v.Size()
	for x := visible.Min.X - visible.Min.X%step; x < visible.Max.X; x = layout.Sum(x, step) {
		tile := l
		if width-x < step {
			tile = l.Truncate(width-x, "")
		}
		tile.Draw(v, x, y)
	}
}

type unit struct {
	cluster string
	// source owns appearance and link identity. Carrying one pointer per cluster
	// instead of copying a Style and string header keeps wrapping proportional to
	// the text model rather than to the size of its presentation vocabulary.
	source *Span
	// linked is false only for a tab expansion. A tab advances through empty cells;
	// unlike an ordinary space, those cells are not painted and therefore cannot be
	// part of a terminal hyperlink.
	linked bool
	width  int
	// space marks a break opportunity. A tab is one, and is also the reason a
	// unit's width is not derivable from its cluster alone.
	space bool
	// at is where this cluster started in the line's text, so that a row can say
	// which part of the line it came from. A tab's expansion carries the tab's own
	// offset on every space it became.
	at int
	// size is how many bytes the cluster took, which is not len(cluster) for a tab
	// that became a space.
	size int
}

func (l Line) units() []unit {
	capacity := 0
	for _, span := range l {
		for _, r := range span.Text {
			capacity = layout.Sum(capacity, 1)
			if r == '\t' {
				capacity = layout.Sum(capacity, TabStop-1)
			}
		}
	}
	units := make([]unit, 0, capacity)
	for u := range l.eachUnit() {
		units = append(units, u)
	}
	return units
}

func (l Line) eachUnit() iter.Seq[unit] {
	return func(yield func(unit) bool) {
		col := 0
		for cluster, s := range l.clusters() {
			switch cluster.Text {
			case "\t":
				n := TabStop - col%TabStop
				for range n {
					if !yield(unit{cluster: " ", source: s, width: 1, space: true, at: cluster.From, size: cluster.To - cluster.From}) {
						return
					}
				}
				col = layout.Sum(col, n)
			default:
				w := grid.ClusterWidth(cluster.Text)
				if !yield(unit{cluster: cluster.Text, source: s, linked: true, width: w, space: cluster.Text == " ", at: cluster.From, size: cluster.To - cluster.From}) {
					return
				}
				col = layout.Sum(col, w)
			}
		}
	}
}

func (l Line) ByteLen() int {
	n := 0
	for _, s := range l {
		n += len(s.Text)
	}
	return n
}

func line(units []unit) Line {
	if len(units) == 0 {
		return Line{}
	}
	// Every non-empty row has one style run. Let uncommon additional runs grow the
	// slice instead of reserving one Span for every grapheme in ordinary plain text.
	out := make(Line, 0, 1)
	previous := ""
	for first := 0; first < len(units); {
		boundary := joins(previous, units[first].cluster)
		last := first + 1
		bytes := len(units[first].cluster)
		for last < len(units) && units[last].sameRun(units[first]) && !joins(units[last-1].cluster, units[last].cluster) {
			bytes += len(units[last].cluster)
			last++
		}
		var text strings.Builder
		text.Grow(bytes)
		for _, u := range units[first:last] {
			text.WriteString(u.cluster)
		}
		out = append(out, Span{
			Text:        text.String(),
			Style:       units[first].style(),
			Link:        units[first].link(),
			BreakBefore: boundary,
		})
		previous = units[last-1].cluster
		first = last
	}
	return out
}

func (u unit) style() grid.Style { return u.source.Style }

func (u unit) link() string {
	if !u.linked {
		return ""
	}
	return u.source.Link
}

func (u unit) sameRun(other unit) bool {
	return u.style() == other.style() && u.link() == other.link()
}

// displayFilter decides, cluster by cluster in line order, which clusters reach a
// cell. Removing a control can leave a zero-width cluster against one that
// segmentation kept it apart from: a variation selector after a newline would
// turn the symbol before the newline into a wider emoji. Such a cluster shows
// nothing of its own, so it is removed with the control instead of being allowed
// to change its neighbor. Measurement is unaffected because it has no width.
type displayFilter struct {
	previous string
	removed  bool
}

func (d *displayFilter) hides(cluster string) bool {
	if dropped(cluster) || d.removed && grid.ClusterWidth(cluster) == 0 && joins(d.previous, cluster) {
		d.removed = true
		return true
	}
	d.previous, d.removed = cluster, false
	return false
}

// joins reports whether segmentation would make next part of previous once the
// two are adjacent.
func joins(previous, next string) bool {
	if previous == "" || next == "" || plainASCIIByte(previous[len(previous)-1]) && plainASCIIByte(next[0]) {
		return false
	}
	return !IsClusterBoundary(previous+next, len(previous))
}

// Display removes controls and malformed bytes while preserving independent
// boundaries and tabs. Its spans may share storage with the source; use Clone at
// ownership boundaries.
//
// It removes rather than replaces, which is where it parts company with [Printable]:
// that one normalizes text somebody will store and hands back a mark where invalid
// bytes were, and this one answers what was drawn, which is nothing.
func (l Line) Display() Line {
	// Long ASCII labels need no segmentation just to establish that every byte
	// survives. Clipping will segment only the prefix it can retain.
	ascii := true
	for _, span := range l {
		if !plainASCII(span.Text) {
			ascii = false
			break
		}
	}
	if ascii {
		return l
	}
	out := make(Line, 0, len(l))
	changed := false
	for _, section := range l.sections() {
		display := projectDisplay(section.String())
		if display.text == display.source {
			out = append(out, section...)
			continue
		}
		changed = true
		var value strings.Builder
		var from *Span
		boundary := section[0].BreakBefore
		end := func() {
			if value.Len() > 0 {
				out = append(out, Span{Text: value.String(), Style: from.Style, Link: from.Link, BreakBefore: boundary})
				value.Reset()
				boundary = false
			}
		}
		for cluster, source := range styledClusters(section, display) {
			if source != from {
				end()
				from = source
			}
			value.WriteString(cluster.Text)
		}
		end()
	}
	if !changed {
		return l
	}
	return out
}

// StampLink converts source byte ranges to terminal columns so wide clusters
// and preceding tabs cannot shift a link onto unrelated text. The returned
// width lets the caller use those same columns for hit testing.
func StampLink(v grid.View, x, y int, s string, start, end int, url string) (col, width int) {
	if start < 0 || end > len(s) || start >= end {
		return 0, 0
	}
	col = ColumnOf(s, start)
	width = ColumnOf(s, end) - col
	v.Link(layout.Translate(x, col), y, width, url)
	return col, width
}
