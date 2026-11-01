package markdown

import (
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/text"
)

// Block is one immutable piece of a rendered document.
//
// A block keeps source and layout semantics together until its final width is
// known. That distinction matters for more than wrapping: a thematic break
// stretches, and a table allocates columns or becomes records when columns stop
// being readable. Exposing pre-laid-out lines would make those decisions too early.
//
// Blocks come from [Render]. Their zero value is empty. They can be
// retained, copied, measured and drawn independently; [Doc] is the convenient way
// to compose them with their inter-block spacing. Extension renderers must uphold
// the same stability contract: published children cannot be mutated or released
// while retained blocks still refer to them.
type Block struct {
	lines   []text.Line
	table   *table
	content grid.Drawable

	prefix blockPrefix
	rule   *text.Span

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
// width. Markers, quotation rails and thematic dividers occupy cells without adding searchable text.
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
	rows                []text.Wrapped
	child               grid.Drawable
	rule                *text.Span
	left, width, height int
	prefix              blockPrefix
}

func (b Block) layout(width int) blockLayout {
	if width <= 0 {
		return blockLayout{}
	}
	if b.rule != nil {
		left := min(b.prefix.indent, width)
		return blockLayout{rule: b.rule, left: left, width: width - left, height: 1, prefix: b.prefix}
	}
	if b.content == nil {
		rows := b.wrapped(width)
		return blockLayout{rows: rows, left: b.prefix.indent, width: width, height: len(rows), prefix: b.prefix}
	}
	left := min(b.prefix.indent, width)
	room := width - left
	height := 0
	if room > 0 {
		height = max(0, b.content.HeightForWidth(room))
	}
	return blockLayout{child: b.content, left: left, width: room, height: height, prefix: b.prefix}
}

// Nested blocks share the immutable quotation rails. Empty list indentation
// only needs a column count; only the block that shows a marker retains it.
type blockPrefix struct {
	indent         int
	rails, markers *prefixMark
}

type prefixMark struct {
	column int
	span   text.Span
	next   *prefixMark
}

func (m *prefixMark) draw(v grid.View, y int) {
	for ; m != nil; m = m.next {
		text.Line{m.span}.Draw(v, m.column, y)
	}
}

func (p blockLayout) draw(v grid.View) {
	if p.child != nil {
		p.child.Draw(v.Sub(grid.Area(p.left, 0, p.width, p.height)))
	}
	visible := v.Visible()
	for y := max(0, visible.Min.Y); y < min(p.height, visible.Max.Y); y++ {
		p.prefix.rails.draw(v, y)
		if y == 0 {
			p.prefix.markers.draw(v, y)
		}
		if p.rule != nil {
			drawRule(v.Sub(grid.Area(p.left, y, p.width, 1)), *p.rule)
		} else if p.child == nil {
			p.rows[y].Draw(v, p.left, y)
		}
	}
}

func (p blockLayout) project() []text.Row {
	if p.rule != nil {
		return []text.Row{{Offset: p.left}}
	}
	if p.child == nil {
		return publicRows(p.rows, p.left)
	}
	rows := make([]text.Row, p.height)
	if projector, ok := p.child.(interface{ Rows(width int) []text.Row }); ok {
		copy(rows, projector.Rows(p.width))
	}
	for i := range rows {
		rows[i].Offset = layout.Translate(rows[i].Offset, p.left)
	}
	return rows
}

func (b Block) wrapped(width int) []text.Wrapped {
	room := max(layout.Remaining(width, b.prefix.indent), 1)
	switch {
	case b.table != nil:
		return b.table.appendRows(nil, room)
	default:
		var rows []text.Wrapped
		for _, line := range b.lines {
			rows = append(rows, line.Wrap(room)...)
		}
		return rows
	}
}

func publicRows(rows []text.Wrapped, offset int) []text.Row {
	out := make([]text.Row, len(rows))
	for i, row := range rows {
		out[i] = text.Row{
			Text: row.Line.String(), Offset: offset,
			Joined: row.Joined, Gap: row.Gap,
		}
	}
	return out
}

// Repetition stays anchored to the logical origin even when a viewport clips its left edge.
func drawRule(v grid.View, glyph text.Span) {
	unit := text.Width(glyph.Text)
	if unit == 0 || v.Empty() {
		return
	}
	width, _ := v.Size()
	visible := v.Visible()
	for x := visible.Min.X - visible.Min.X%unit; x < visible.Max.X; {
		v.Text(x, 0, text.Truncate(glyph.Text, width-x, ""), glyph.Style)
		if visible.Max.X-x <= unit {
			break
		}
		x += unit
	}
}

var _ grid.Drawable = Block{}
