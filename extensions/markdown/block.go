package markdown

import (
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/text"
)

// Block defers layout to the final width because tables may need records and dividers
// may need to stretch. Blocks and their retained extension content remain immutable
// so copies can be measured, drawn and selected independently.
type Block struct {
	lines   []text.Line
	table   *table
	content grid.Drawable

	prefix blockPrefix
	rule   *text.Span

	blankBefore bool
}

// HeightForWidth excludes inter-block spacing because only the composer knows
// whether another block precedes this one.
func (b Block) HeightForWidth(width int) int { return b.layout(width).height }

func (b Block) Draw(v grid.View) {
	if v.Empty() {
		return
	}
	width, _ := v.Size()
	b.layout(width).draw(v)
}

// Rows preserves marker, quotation-rail and divider geometry separately because
// those cells affect selection offsets without becoming searchable content.
func (b Block) Rows(width int) []text.Row { return b.layout(width).project() }

// BlankBefore leaves spacing to the composer because only it knows whether a
// preceding block exists; a block cannot declare itself the first visible block.
func (b Block) BlankBefore() bool { return b.blankBefore }

// An embedded drawable keeps its own geometry so text projection cannot flatten
// its drawing contract.
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
	// Even a deeply nested block needs a content column. Clip its furniture to
	// the same inset used for drawing, wrapping and semantic row offsets.
	left := min(b.prefix.indent, width-1)
	room := width - left
	placed := blockLayout{left: left, width: room, prefix: b.prefix}
	switch {
	case b.rule != nil:
		placed.rule, placed.height = b.rule, 1
	case b.content != nil:
		placed.child, placed.height = b.content, max(0, b.content.HeightForWidth(room))
	default:
		placed.rows = b.wrapped(room)
		placed.height = len(placed.rows)
	}
	return placed
}

// Nested blocks share the immutable quotation rails; list items own their markers.
type blockPrefix struct {
	indent int
	rails  *prefixMark
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
	inset := v.Sub(grid.Area(0, 0, p.left, p.height))
	for y := max(0, visible.Min.Y); y < min(p.height, visible.Max.Y); y++ {
		p.prefix.rails.draw(inset, y)
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

func (b Block) wrapped(room int) []text.Wrapped {
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
