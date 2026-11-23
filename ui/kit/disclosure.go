package kit

import (
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/text"
)

// Disclosure is a summary that opens onto details. A marker before the summary says
// whether there is anything to open and whether it is open, and the details draw
// under the summary, aligned with it, only while Open. What opens it is its owner's
// state; the disclosure itself is passive.
//
// The marker decorates rather than says anything, so it projects no text: copying a
// disclosure reads its summary and, while open, its details.
type Disclosure struct {
	Theme  Theme
	Glyphs Glyphs
	// Summary is always shown, usually one [Split] row naming the thing and its state.
	Summary grid.Drawable
	// Details are shown under the summary while Open. Nil means there is nothing to
	// open, which the marker shows with Glyphs.Bullet and Open cannot change.
	Details grid.Drawable
	Open    bool
}

var _ text.Projector = Disclosure{}

func (d Disclosure) HeightForWidth(width int) int { return d.block().HeightForWidth(width) }

func (d Disclosure) Draw(v grid.View) {
	width, height := v.Size()
	if width <= 0 || height <= 0 {
		return
	}
	d.block().Draw(v)
	marker, style := d.marker()
	text.Of(marker, style).Truncate(width, "").Draw(v, 0, 0)
}

func (d Disclosure) Rows(width int) []text.Row { return d.block().Rows(width) }

func (d Disclosure) opens() bool { return !grid.IsNilDrawable(d.Details) }

func (d Disclosure) expanded() bool { return d.Open && d.opens() }

func (d Disclosure) block() Stack {
	inset := d.inset()
	summary := Indent{Body: d.Summary, Left: inset}
	if !d.expanded() {
		return Stack{summary}
	}
	return Stack{summary, Indent{Body: d.Details, Left: inset}}
}

func (d Disclosure) marker() (string, grid.Style) {
	switch {
	case d.expanded():
		return d.Glyphs.Expanded, d.Theme.Accent
	case d.opens():
		return d.Glyphs.Collapsed, d.Theme.Muted
	default:
		return d.Glyphs.Bullet, d.Theme.Muted
	}
}

// inset reserves the widest marker and a space after it, so opening or emptying
// the details never moves the summary sideways.
func (d Disclosure) inset() int {
	widest := max(text.Width(d.Glyphs.Expanded), text.Width(d.Glyphs.Collapsed), text.Width(d.Glyphs.Bullet))
	if widest == 0 {
		return 0
	}
	return layout.Sum(widest, 1)
}
