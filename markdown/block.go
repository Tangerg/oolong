package markdown

import (
	"strings"

	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/layout"
	"github.com/Tangerg/oolong/core/text"
)

// Block is one immutable piece of a rendered document.
//
// A block keeps source and layout semantics together until its final width is
// known. That distinction matters for more than wrapping: a thematic break
// stretches, and a table allocates columns or becomes records when columns stop
// being readable. Exposing pre-laid-out lines would make those decisions too early.
//
// Blocks come from [Render] and [Stream]. Their zero value is empty. They can be
// retained, copied, measured and drawn independently; [Doc] is the convenient way
// to compose them with their inter-block spacing. Extension renderers must uphold
// the same stability contract: published children cannot be mutated or released
// while retained blocks still refer to them.
type Block struct {
	lines   []text.Line
	table   *table
	content grid.Drawable

	indent int
	marker text.Line
	// markerDepth is how many rail segments stand outside the marker. See
	// [withMarker].
	markerDepth int
	rail        text.Line
	rule        bool

	blankBefore bool
}

// HeightForWidth reports how many rows the block needs at width. It excludes the blank
// row that may separate this block from the one before it; [Block.BlankBefore]
// exposes that relationship to custom composers.
func (b Block) HeightForWidth(width int) int { return b.layout(width).height }

// Draw writes the block into v. It excludes any blank row before the block, because
// only the composer knows whether a preceding block exists.
func (b Block) Draw(v grid.View) {
	if v.Empty() {
		return
	}
	width, _ := v.Size()
	b.layout(width).draw(v)
}

// Rows returns the meaningful text and offsets of the block's physical rows at
// width. Markers and quotation rails are decoration and are not included in Text.
func (b Block) Rows(width int) []text.Row { return b.layout(width).project() }

// BlankBefore reports whether this block wants a blank row in front of it: false
// between the items of a tight list, true where separate prose blocks need to read as
// separate things.
//
// It is the block's own spacing and not a statement about the document. Whether
// anything precedes this block is the composer's to know — the same reason
// [Block.Draw] leaves the row out — and a stream settles its answer one piece at a
// time, so no block can say whether it is the first one anybody will see.
func (b Block) BlankBefore() bool { return b.blankBefore }

// blockLayout keeps an embedded child's geometry without flattening its drawing.
type blockLayout struct {
	rows                []row
	child               grid.Drawable
	left, width, height int
	rail, marker        text.Line
	markerDepth         int
}

func (b Block) layout(width int) blockLayout {
	if width <= 0 {
		return blockLayout{}
	}
	if b.content == nil {
		rows := b.appendRows(nil, width)
		return blockLayout{rows: rows, width: width, height: len(rows)}
	}
	left := min(max(b.indent, 0), width)
	room := width - left
	height := 0
	if room > 0 {
		height = max(0, b.content.HeightForWidth(room))
	}
	return blockLayout{child: b.content, left: left, width: room, height: height, rail: b.rail, marker: b.marker, markerDepth: b.markerDepth}
}

// withMarker is the first row's prefix: the quotation bars the item sits inside, the
// mark that begins it, and then the bars of whatever the item itself quotes.
//
// The bars and the mark do not occupy the same columns, which is what replacing one
// with the other got wrong: every item of a quoted list lost its bar and the list
// drifted left by the width of it. Nor are they in a fixed order — a list in a
// quotation is a bar then a bullet, and a quotation in a list item is a bullet then a
// bar — which is why the mark is placed by how many bars stood outside the list it
// begins an item of.
func withMarker(rail, marker text.Line, depth, indent int) text.Line {
	if len(marker) == 0 {
		return rail
	}
	depth = min(max(depth, 0), len(rail))
	out := make(text.Line, 0, len(rail)+len(marker)+1)
	out = append(out, rail[:depth]...)
	// Whatever the indent holds beyond the bars and this mark belongs to the lists
	// this item is nested inside. Their marks were drawn on their own first rows and
	// this one stands to the right of the room they took.
	if gap := indent - rail.Width() - marker.Width(); gap > 0 {
		out = append(out, text.Span{Text: strings.Repeat(" ", gap)})
	}
	out = append(out, marker...)
	return append(out, rail[depth:]...)
}

// alongsideMarker is the prefix of every row after the first: the same bars in the
// same columns, with blanks where the mark stood.
//
// A rail belongs to the quotation and stands in the quotation's columns, which is
// what hanging it off the left of each row's own text got wrong. Only the first row
// was padded out to the item's indent, so a quoted list item that wrapped drew its
// bar underneath its own second line — two columns in, and further in for every level
// of nesting.
func alongsideMarker(rail, marker text.Line, depth, indent int) text.Line {
	width := marker.Width()
	if len(rail) == 0 || width <= 0 {
		return rail
	}
	return withMarker(rail, text.Line{{Text: strings.Repeat(" ", width)}}, depth, indent)
}

func (p blockLayout) draw(v grid.View) {
	if p.child == nil {
		drawRows(v, p.rows)
		return
	}
	p.child.Draw(v.Sub(grid.Area(p.left, 0, p.width, p.height)))
	visible := v.Visible()
	continued := alongsideMarker(p.rail, p.marker, p.markerDepth, p.left)
	for y := max(0, visible.Min.Y); y < min(p.height, visible.Max.Y); y++ {
		prefix := continued
		if y == 0 && len(p.marker) > 0 {
			prefix = withMarker(p.rail, p.marker, p.markerDepth, p.left)
		}
		prefix.Draw(v, 0, y)
	}
}

func (p blockLayout) project() []text.Row {
	if p.child == nil {
		return publicRows(p.rows)
	}
	rows := make([]text.Row, p.height)
	if projector, ok := p.child.(interface{ Rows(width int) []text.Row }); ok {
		copy(rows, projector.Rows(p.width))
	}
	for i := range rows {
		rows[i].Offset += p.left
	}
	return rows
}

// row is one physical row: what it says, where it starts, and the decoration that
// ends immediately before it.
type row struct {
	text.Wrapped
	at     int
	prefix text.Line
	gap    string
}

func (b Block) appendRows(dst []row, width int) []row {
	start := len(dst)
	at := max(b.indent, 0)
	room := max(layout.Remaining(width, at), 1)
	switch {
	case b.table != nil:
		dst = b.table.appendRows(dst, room)
	case b.rule:
		dst = append(dst, row{Line: stretch(b.lines, room)})
	default:
		for _, line := range b.lines {
			dst = appendWrapped(dst, line, room)
		}
	}

	for i := start; i < len(dst); i++ {
		dst[i].at = at
		dst[i].prefix = alongsideMarker(b.rail, b.marker, b.markerDepth, b.indent)
	}
	if start < len(dst) && len(b.marker) > 0 {
		dst[start].prefix = withMarker(b.rail, b.marker, b.markerDepth, b.indent)
	}
	return dst
}

func appendWrapped(dst []row, line text.Line, width int) []row {
	whole := line.String()
	previous := 0
	for _, wrapped := range line.Wrap(width) {
		gap := ""
		if wrapped.Joined && previous <= wrapped.From && wrapped.From <= len(whole) {
			gap = whole[previous:wrapped.From]
		}
		dst = append(dst, row{Wrapped: wrapped, gap: gap})
		previous = wrapped.To
	}
	return dst
}

func drawRows(v grid.View, rows []row) {
	visible := v.Visible()
	first := min(max(visible.Min.Y, 0), len(rows))
	last := min(max(visible.Max.Y, first), len(rows))
	for y := first; y < last; y++ {
		r := rows[y]
		if len(r.prefix) > 0 {
			// At the left edge and not against the text: the bars are the quotation's
			// columns, and every row of the block is inside the same quotation.
			r.prefix.Draw(v, 0, y)
		}
		r.Draw(v, r.at, y)
	}
}

func publicRows(rows []row) []text.Row {
	out := make([]text.Row, len(rows))
	for i, row := range rows {
		out[i] = text.Row{
			Text: row.Line.String(), Offset: row.at,
			Joined: row.Joined, Gap: row.gap,
		}
	}
	return out
}

// stretch repeats a line until it fills the room there is, which is what a rule
// across the page is: one character, as many times as the width says.
func stretch(lines []text.Line, room int) text.Line {
	if len(lines) == 0 || len(lines[0]) == 0 {
		return nil
	}
	span := lines[0][0]
	width := text.Width(span.Text)
	if width <= 0 {
		return nil
	}
	span.Text = strings.Repeat(span.Text, room/width+1)
	return text.Line{span}.Truncate(room, "")
}

var _ grid.Drawable = Block{}
