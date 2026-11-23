package kit

import (
	"slices"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/text"
)

// Stack draws passive blocks one under another, each at the stack's full width. Its
// text projection puts every block's rows where Draw puts the block, and gives a block
// that projects no text blank rows, so a selection over the composition reads exactly
// what it shows. A nil block takes no rows.
type Stack []grid.Drawable

// Indent draws Body Left cells in from the left edge, and shifts Body's text rows by
// the same amount so a selection lines up with the indented text. Like a
// [Paragraph]'s indent, Body keeps at least one column: at a width no wider than Left
// it is clipped rather than reflowed into the indent. A negative Left indents nothing.
type Indent struct {
	Body grid.Drawable
	Left int
	// Rule is drawn in RuleStyle in the indent's first column beside every row Body
	// takes: a quote bar, or the rail that ties a block's details to its header. It
	// decorates rather than says anything, so it projects no text.
	Rule      CellGlyph
	RuleStyle grid.Style
}

// Gap is blank rows, such as the space that separates one transcript entry from the next.
type Gap int

// Framed is Body inside Box as one passive block, for a frame that belongs to what
// it holds, such as a block set apart on a surface of its own. It measures, draws and
// projects text as the two laid out together: the frame takes its rows and columns
// and projects nothing, and Body's rows sit where Body is drawn. Like [Indent],
// Body keeps at least one column, clipped rather than reflowed into the frame.
type Framed struct {
	Box  Box
	Body grid.Drawable
}

var (
	_ text.Projector = Stack(nil)
	_ text.Projector = Indent{}
	_ text.Projector = Gap(0)
	_ text.Projector = Framed{}
)

func (s Stack) HeightForWidth(width int) int {
	total := 0
	for _, block := range s {
		total = layout.Sum(total, blockRows(block, width))
	}
	return total
}

func (s Stack) Draw(v grid.View) {
	width, height := v.Size()
	visible := v.Visible()
	if width <= 0 || height <= 0 || visible.Empty() {
		return
	}
	top := 0
	for _, block := range s {
		if top >= visible.Max.Y {
			return
		}
		rows := blockRows(block, width)
		if rows > 0 && layout.Sum(top, rows) > visible.Min.Y {
			block.Draw(v.Sub(grid.Area(0, top, width, rows)))
		}
		top = layout.Sum(top, rows)
	}
}

func (s Stack) Rows(width int) []text.Row {
	out := make([]text.Row, 0, s.HeightForWidth(width))
	for _, block := range s {
		out = append(out, projectedRows(block, width)...)
	}
	return out
}

func (i Indent) HeightForWidth(width int) int { return blockRows(i.Body, i.bodyWidth(width)) }

func (i Indent) Draw(v grid.View) {
	width, height := v.Size()
	if grid.IsNilDrawable(i.Body) || width <= 0 || height <= 0 {
		return
	}
	i.Body.Draw(v.Sub(grid.Area(max(i.Left, 0), 0, i.bodyWidth(width), height)))
	if i.Rule.value == "" || i.Left <= 0 {
		return
	}
	visible := v.Visible()
	for y := visible.Min.Y; y < min(visible.Max.Y, i.HeightForWidth(width)); y++ {
		v.Text(0, y, i.Rule.value, i.RuleStyle)
	}
}

func (i Indent) Rows(width int) []text.Row {
	rows := projectedRows(i.Body, i.bodyWidth(width))
	for at := range rows {
		rows[at].Offset = layout.Sum(rows[at].Offset, i.Left)
	}
	return rows
}

func (i Indent) bodyWidth(width int) int { return max(layout.Remaining(width, i.Left), 1) }

func (f Framed) HeightForWidth(width int) int {
	insets := f.Box.insets()
	return layout.Sum(f.interior(insets).HeightForWidth(layout.Remaining(width, insets.Right)), insets.Top, insets.Bottom)
}

// Draw panics for an invalid title or footer alignment before changing cells.
func (f Framed) Draw(v grid.View) {
	interior := f.Box.Draw(v)
	if !grid.IsNilDrawable(f.Body) && !interior.Empty() {
		f.Body.Draw(interior)
	}
}

func (f Framed) Rows(width int) []text.Row {
	insets := f.Box.insets()
	body := f.interior(insets).Rows(layout.Remaining(width, insets.Right))
	return slices.Concat(make([]text.Row, max(insets.Top, 0)), body, make([]text.Row, max(insets.Bottom, 0)))
}

// interior is Body clear of the frame's left edge; measuring it without the right
// edge leaves Body exactly the columns Box.Draw hands it.
func (f Framed) interior(insets layout.Inset) Indent { return Indent{Body: f.Body, Left: insets.Left} }

func (g Gap) HeightForWidth(int) int { return max(int(g), 0) }

func (Gap) Draw(grid.View) {}

func (g Gap) Rows(int) []text.Row { return make([]text.Row, g.HeightForWidth(0)) }

// blockRows is how tall a composed block draws at width; absent content takes no rows.
func blockRows(block grid.Drawable, width int) int {
	if grid.IsNilDrawable(block) {
		return 0
	}
	return max(block.HeightForWidth(width), 0)
}

// projectedRows is a block's text projection aligned to the rows it draws.
func projectedRows(block grid.Drawable, width int) []text.Row {
	return text.ProjectedRows(block, width, blockRows(block, width))
}
