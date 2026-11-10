package kit

import (
	"fmt"
	"image"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/text"
)

// Box frames and pads a region.
//
// It is not a container: it does not own a child. A caller draws the box and then
// draws into the view the box hands back, which keeps the box out of the question
// of what goes inside it and lets the same box frame a widget, a string, or nothing.
// A focusable child that must participate in routing inside the frame belongs in a
// [Panel], which owns that narrower live composition explicitly.
//
// Border overrides the frame drawn from Glyphs; a box with neither glyphs nor a
// border has no frame, and therefore no room for a title or footer.
type Box struct {
	Theme   Theme
	Glyphs  Glyphs
	Border  Border
	Padding layout.Inset
	// Title sits in the top border, inset so the corner reads as a corner.
	Title       string
	TitleAlign  layout.Align
	Footer      string
	FooterAlign layout.Align
}

func (b Box) border() Border {
	if b.Border != (Border{}) {
		return b.Border
	}
	return b.Glyphs.Rounded()
}

// Overhead is how many columns and rows the frame and padding take, which is what a
// caller subtracts to know what is left for the content.
func (b Box) Overhead() image.Point {
	return b.insets().Size()
}

func (b Box) insets() layout.Inset {
	edge := 0
	if b.border().drawn() {
		edge = 1
	}
	return layout.Inset{
		Top: layout.Sum(edge, b.Padding.Top), Bottom: layout.Sum(edge, b.Padding.Bottom),
		Left: layout.Sum(edge, b.Padding.Left), Right: layout.Sum(edge, b.Padding.Right),
	}
}

func (b Box) InnerRect(size image.Point) image.Rectangle {
	if size.X <= 0 || size.Y <= 0 {
		// A box with no room has no interior, and saying so here is what keeps this
		// the same answer [Box.Draw] gives. Reporting an origin inside a region that
		// does not exist would leave the two disagreeing about a rectangle neither
		// can draw in.
		return image.Rectangle{}
	}
	insets := b.insets()
	over := insets.Size()
	inner := image.Pt(layout.Remaining(size.X, over.X), layout.Remaining(size.Y, over.Y))
	x, y := 0, 0
	if inner.X > 0 {
		x = insets.Left
	}
	if inner.Y > 0 {
		y = insets.Top
	}
	return grid.Area(x, y, inner.X, inner.Y)
}

// Draw paints the frame and returns the region left for content, so the common use
// reads as one step:
//
//	inner := box.Draw(v)
//	content.Draw(inner)
//
// Invalid TitleAlign or FooterAlign panics before any cells change, even when
// labels or the view are empty, so rejection cannot leave a partial frame.
func (b Box) Draw(v grid.View) grid.View {
	b.paint(v)
	return v.Sub(b.InnerRect(v.Bounds().Size()))
}

// paint draws only the appearance. Live compositions use it after computing the
// interior rectangle they must stage with a base frame; Draw remains the public
// one-step path for passive content.
func (b Box) paint(v grid.View) {
	if err := b.validate(); err != nil {
		panic(err)
	}
	if v.Empty() {
		return
	}
	w, h := v.Size()
	if fill := b.Theme.Surface; fill != (grid.Style{}) {
		v.Fill(grid.Area(0, 0, w, h), fill)
	}
	if border := b.border(); border.drawn() {
		b.drawBorder(v, border, w, h)
	}
}

func (b Box) validate() error {
	if err := b.TitleAlign.Validate(); err != nil {
		return fmt.Errorf("kit: box title: %w", err)
	}
	if err := b.FooterAlign.Validate(); err != nil {
		return fmt.Errorf("kit: box footer: %w", err)
	}
	return nil
}

func (b Box) drawBorder(v grid.View, border Border, w, h int) {
	visible := v.Visible()
	// A box one column or one row deep has no room for two opposing edges. Drawing
	// what fits and no more keeps a collapsing layout from looking corrupted.
	if 0 >= visible.Min.Y && 0 < visible.Max.Y {
		b.drawEdge(v, 0, w, border.TopLeft, border.Top, border.TopRight)
		b.label(v, 0, w, b.Title, b.Theme.Heading, b.TitleAlign)
	}
	bottom := h - 1
	if h >= 2 && bottom >= visible.Min.Y && bottom < visible.Max.Y {
		b.drawEdge(v, h-1, w, border.BottomLeft, border.Bottom, border.BottomRight)
		b.label(v, bottom, w, b.Footer, b.Theme.Subtle, b.FooterAlign)
	}
	first := max(1, visible.Min.Y)
	last := min(h-1, visible.Max.Y)
	for y := first; y < last; y++ {
		v.Text(0, y, border.Left.value, b.Theme.Border)
		if w >= 2 {
			v.Text(w-1, y, border.Right.value, b.Theme.Border)
		}
	}
}

func (b Box) drawEdge(v grid.View, y, w int, left, mid, right CellGlyph) {
	visible := v.Visible()
	if visible.Min.X <= 0 && visible.Max.X > 0 {
		v.Text(0, y, left.value, b.Theme.Border)
	}
	first := max(1, visible.Min.X)
	last := min(w-1, visible.Max.X)
	for x := first; x < last; x++ {
		v.Text(x, y, mid.value, b.Theme.Border)
	}
	if w >= 2 && w-1 >= visible.Min.X && w-1 < visible.Max.X {
		v.Text(w-1, y, right.value, b.Theme.Border)
	}
}

const borderLabelInset = 2

// The corner and an adjacent border cell must remain visible on each side.
func (b Box) label(v grid.View, y, w int, label string, style grid.Style, align layout.Align) {
	room := w - 2*borderLabelInset
	if label == "" || room <= 0 {
		return
	}
	shown := text.Of(label, style).Truncate(room, b.Glyphs.Ellipsis)
	x := borderLabelInset + align.Offset(room, shown.Width())
	shown.Draw(v, x, y)
}

func (b Box) clone() Box {
	b.Glyphs = b.Glyphs.clone()
	return b
}
