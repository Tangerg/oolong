package kit

import (
	"image"

	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/layout"
	"github.com/Tangerg/oolong/core/text"
)

// Box frames and pads a region.
//
// It is not a container: it does not own a child. A caller draws the box and then
// draws into the view the box hands back, which keeps the box out of the question
// of what goes inside it and lets the same box frame a widget, a string, or nothing.
// A focusable child that must participate in routing inside the frame belongs in a
// [Panel], which owns that narrower live composition explicitly.
type Box struct {
	// Theme is the look. A box's parts each have a fixed role in one — the frame is
	// a border, the interior is a surface, the title is a heading — so there is
	// nothing here for a caller to choose between and no style of its own to set.
	Theme Theme
	// Glyphs are the characters the frame is drawn with, which is a fact about the
	// terminal rather than about the look: a box drawn with line characters on a
	// terminal that cannot show them is a box drawn in mojibake.
	Glyphs Glyphs
	// Border is which frame to draw. The zero value takes the rounded one from the
	// glyph set, which reads as a panel; [Box.Bare] draws none.
	Border Border
	// Padding is held clear inside the border.
	Padding layout.Inset
	// Bare draws no frame at all, for a box that only pads and fills.
	Bare bool
	// Title sits in the top border, indented one column so the corner reads as a
	// corner.
	Title      string
	TitleAlign layout.Align
	// Footer sits in the bottom border, on the same terms as the title.
	Footer      string
	FooterAlign layout.Align
}

// border is the frame this box actually draws.
func (b Box) border() Border {
	if b.Bare {
		return Border{}
	}
	if b.Border != (Border{}) {
		return b.Border
	}
	return b.Glyphs.Rounded()
}

// Overhead is how many columns and rows the frame and padding take, which is what a
// caller subtracts to know what is left for the content.
func (b Box) Overhead() image.Point {
	edge := 0
	if b.border().drawn() {
		edge = 2
	}
	pad := b.Padding.Size()
	return image.Pt(layout.Sum(edge, pad.X), layout.Sum(edge, pad.Y))
}

// Inner is the region left for content, in v's coordinates.
func (b Box) Inner(v grid.View) grid.View {
	return v.Sub(b.InnerRect(v.Bounds().Size()))
}

// InnerRect is the region left for content within a space of size.
func (b Box) InnerRect(size image.Point) image.Rectangle {
	if size.X <= 0 || size.Y <= 0 {
		// A box with no room has no interior, and saying so here is what keeps this
		// the same answer [Box.Draw] gives. Reporting an origin inside a region that
		// does not exist would leave the two disagreeing about a rectangle neither
		// can draw in.
		return image.Rectangle{}
	}
	edge := 0
	if b.border().drawn() {
		edge = 1
	}
	over := b.Overhead()
	inner := image.Pt(layout.Remaining(size.X, over.X), layout.Remaining(size.Y, over.Y))
	x, y := 0, 0
	if inner.X > 0 {
		// A positive remainder proves this addition fits: Overhead already includes
		// both sides and is strictly smaller than size on this axis.
		x = edge + b.Padding.Left
	}
	if inner.Y > 0 {
		y = edge + b.Padding.Top
	}
	return grid.Rect(x, y, inner.X, inner.Y)
}

// Draw paints the frame and returns the region left for content, so the common use
// reads as one step:
//
//	inner := box.Draw(v)
//	content.Draw(inner)
func (b Box) Draw(v grid.View) grid.View {
	b.paint(v)
	return b.Inner(v)
}

// paint draws only the appearance. Live compositions use it after computing the
// interior rectangle they must stage with a headless frame; Draw remains the public
// one-step path for passive content.
func (b Box) paint(v grid.View) {
	if v.Empty() {
		return
	}
	w, h := v.Size()
	if w <= 0 || h <= 0 {
		return
	}
	if fill := b.Theme.Surface; fill != (grid.Style{}) {
		v.Fill(grid.Rect(0, 0, w, h), fill)
	}
	if border := b.border(); border.drawn() {
		b.drawBorder(v, border, w, h)
	}
}

func (b Box) drawBorder(v grid.View, border Border, w, h int) {
	visible := v.Visible()
	// A box one column or one row deep has no room for two opposing edges. Drawing
	// what fits and no more keeps a collapsing layout from looking corrupted.
	if h >= 1 && 0 >= visible.Min.Y && 0 < visible.Max.Y {
		b.drawEdge(v, 0, w, border.TopLeft, border.Top, border.TopRight)
	}
	bottom := h - 1
	if h >= 2 && bottom >= visible.Min.Y && bottom < visible.Max.Y {
		b.drawEdge(v, h-1, w, border.BottomLeft, border.Bottom, border.BottomRight)
	}
	first := max(1, visible.Min.Y)
	last := min(h-1, visible.Max.Y)
	for y := first; y < last; y++ {
		v.Text(0, y, border.Left, b.Theme.Border)
		if w >= 2 {
			v.Text(w-1, y, border.Right, b.Theme.Border)
		}
	}
	if h >= 1 && 0 >= visible.Min.Y && 0 < visible.Max.Y {
		b.label(v, 0, w, b.Title, b.Theme.Heading, b.TitleAlign)
	}
	if h >= 2 && bottom >= visible.Min.Y && bottom < visible.Max.Y {
		b.label(v, h-1, w, b.Footer, b.Theme.Subtle, b.FooterAlign)
	}
}

func (b Box) drawEdge(v grid.View, y, w int, left, mid, right string) {
	visible := v.Visible()
	if visible.Min.X <= 0 && visible.Max.X > 0 {
		v.Text(0, y, left, b.Theme.Border)
	}
	first := max(1, visible.Min.X)
	last := min(w-1, visible.Max.X)
	for x := first; x < last; x++ {
		v.Text(x, y, mid, b.Theme.Border)
	}
	if w >= 2 && w-1 >= visible.Min.X && w-1 < visible.Max.X {
		v.Text(w-1, y, right, b.Theme.Border)
	}
}

// label writes a title or footer into a border row, keeping a column of border on
// each side of it so the line still reads as a frame.
func (b Box) label(v grid.View, y, w int, label string, style grid.Style, align layout.Align) {
	if label == "" || w <= 4 {
		return
	}
	room := w - 4
	label = text.Truncate(label, room, "…")
	width := text.Width(label)
	x := 2 + align.Offset(room, width)
	v.Text(x, y, label, style)
}
