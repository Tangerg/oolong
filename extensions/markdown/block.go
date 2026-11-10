package markdown

import (
	"context"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/text"
)

// Block defers layout to the final width because tables may need records and dividers
// may need to stretch. Blocks and their retained extension content remain immutable
// so copies can be measured, drawn and selected independently.
type Block struct {
	body   blockBody
	prefix blockPrefix

	blankBefore bool
}

// HeightForWidth excludes inter-block spacing because only the composer knows
// whether another block precedes this one.
func (b Block) HeightForWidth(width int) int {
	placed, _ := b.prepareLayout(context.Background(), width)
	return placed.height
}

func (b Block) Draw(v grid.View) {
	if v.Empty() {
		return
	}
	width, _ := v.Size()
	placed, _ := b.prepareLayout(context.Background(), width)
	placed.draw(v)
}

// Rows preserves marker, quotation-rail and divider geometry separately because
// those cells affect selection offsets without becoming searchable content.
func (b Block) Rows(width int) []text.Row {
	ctx := context.Background()
	placed, _ := b.prepareLayout(ctx, width)
	rows, _ := placed.project(ctx)
	return rows
}

// BlankBefore leaves spacing to the composer because only it knows whether a
// preceding block exists; a block cannot declare itself the first visible block.
func (b Block) BlankBefore() bool { return b.blankBefore }

// blockLayout is a block placed at one width: its body in the column right of
// the block's inset, and the quotation rails in that inset on every row.
type blockLayout struct {
	body        placedBody
	left, width int
	height      int
	prefix      blockPrefix
}

func (b Block) prepareLayout(ctx context.Context, width int) (blockLayout, error) {
	if width <= 0 || b.body == nil {
		return blockLayout{}, nil
	}
	// Even a deeply nested block needs a content column. Clip its furniture to
	// the same inset used for drawing, wrapping and semantic row offsets.
	left := min(b.prefix.indent, width-1)
	room := width - left
	body, err := b.body.layout(ctx, room)
	if err != nil {
		return blockLayout{}, err
	}
	return blockLayout{body: body, left: left, width: room, height: body.height(), prefix: b.prefix}, nil
}

// blockBody is what a block holds. Each kind lays itself out at the width the
// block's inset leaves it.
type blockBody interface {
	layout(ctx context.Context, width int) (placedBody, error)
}

// placedBody is a body laid out at one width. It draws into a view the size of
// its own area and reports rows whose offsets start at that area's left edge.
type placedBody interface {
	height() int
	draw(v grid.View)
	project(ctx context.Context) ([]text.Row, error)
}

// textBody is wrapped paragraphs and headings.
type textBody []text.Line

func (b textBody) layout(_ context.Context, width int) (placedBody, error) {
	var rows wrappedRows
	for _, line := range b {
		rows = append(rows, line.Wrap(width)...)
	}
	return rows, nil
}

// table lays out as wrapped rows too; its records and dividers decide where.
func (t *table) layout(_ context.Context, width int) (placedBody, error) {
	return wrappedRows(t.appendRows(nil, width)), nil
}

type wrappedRows []text.Wrapped

func (r wrappedRows) height() int { return len(r) }

func (r wrappedRows) draw(v grid.View) {
	visible := v.Visible()
	for y := max(0, visible.Min.Y); y < min(len(r), visible.Max.Y); y++ {
		r[y].Draw(v, 0, y)
	}
}

func (r wrappedRows) project(context.Context) ([]text.Row, error) {
	return publicRows(r, 0), nil
}

// embeddedBody is an extension's drawable. It keeps its own geometry so text
// projection cannot flatten its drawing contract.
type embeddedBody struct{ child grid.Drawable }

func (b embeddedBody) layout(_ context.Context, width int) (placedBody, error) {
	return placedEmbedded{child: b.child, width: width, rows: max(0, b.child.HeightForWidth(width))}, nil
}

type placedEmbedded struct {
	child       grid.Drawable
	width, rows int
}

func (p placedEmbedded) height() int { return p.rows }

func (p placedEmbedded) draw(v grid.View) { p.child.Draw(v) }

func (p placedEmbedded) project(context.Context) ([]text.Row, error) {
	rows := make([]text.Row, p.rows)
	if projector, ok := p.child.(text.Projector); ok {
		copy(rows, text.CloneRows(projector.Rows(p.width)))
	}
	return rows, nil
}

// ruleBody is a thematic break: one row of a repeated glyph.
type ruleBody struct{ glyph text.Span }

func (b ruleBody) layout(context.Context, int) (placedBody, error) { return b, nil }

func (ruleBody) height() int { return 1 }

func (b ruleBody) draw(v grid.View) { drawRule(v, b.glyph) }

func (ruleBody) project(context.Context) ([]text.Row, error) { return []text.Row{{}}, nil }

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
	if p.body == nil {
		return
	}
	p.body.draw(v.Sub(grid.Area(p.left, 0, p.width, p.height)))
	visible := v.Visible()
	inset := v.Sub(grid.Area(0, 0, p.left, p.height))
	for y := max(0, visible.Min.Y); y < min(p.height, visible.Max.Y); y++ {
		p.prefix.rails.draw(inset, y)
	}
}

func (p blockLayout) project(ctx context.Context) ([]text.Row, error) {
	if p.body == nil {
		return nil, nil
	}
	rows, err := p.body.project(ctx)
	if err != nil {
		return nil, err
	}
	for i := range rows {
		rows[i].Offset = layout.Translate(rows[i].Offset, p.left)
	}
	return rows, nil
}

func publicRows(rows []text.Wrapped, offset int) []text.Row {
	out := make([]text.Row, len(rows))
	for i, row := range rows {
		out[i] = text.Row{
			Content: row.Line.Clone(), Offset: offset,
			Joined: row.Joined, Gap: row.Gap,
		}
	}
	return out
}

// Repetition stays anchored to the logical origin even when a viewport clips its left edge.
func drawRule(v grid.View, glyph text.Span) {
	pattern := text.Line{glyph}
	unit := pattern.Width()
	if unit == 0 || v.Empty() {
		return
	}
	width, _ := v.Size()
	visible := v.Visible()
	for x := visible.Min.X - visible.Min.X%unit; x < visible.Max.X; {
		pattern.Truncate(width-x, "").Draw(v, x, 0)
		if visible.Max.X-x <= unit {
			break
		}
		x += unit
	}
}

var _ grid.Drawable = Block{}
