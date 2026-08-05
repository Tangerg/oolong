package kit

import (
	"image"

	"github.com/Tangerg/oolong/primitives/grid"
	"github.com/Tangerg/oolong/primitives/layout"
)

// Overlay floats one thing over another.
//
// It is placement and dimming, not a container: it works out where a layer goes and
// hands back the view to draw it in. What goes inside is the caller's, which is what
// lets the same overlay carry a dialog, a completion list, or a single line of
// warning.
//
// Where the layer goes is [layout.Placement]'s answer and nothing is added to it
// here but the part that has an appearance. A hit test that needs the same rectangle
// a frame later asks the placement, which is why that lives a layer down and holds
// no styling of its own.
type Overlay struct {
	layout.Placement
	// Shade dims what the layer covers, so the eye goes to the layer and it is
	// obvious that what is behind it is not the thing to act on. Its zero value
	// covers nothing.
	Shade grid.Style
}

// Draw dims what is behind the layer and returns the view to draw the layer into.
func (o Overlay) Draw(v grid.View) grid.View {
	width, height := v.Size()
	if width <= 0 || height <= 0 {
		return v.Sub(image.Rectangle{})
	}
	if o.Shade != (grid.Style{}) {
		// Restyled rather than filled: what is behind stays legible and simply
		// recedes, which is what tells the reader it is still there and not gone.
		o.shade(v, width, height)
	}
	return v.Sub(o.Area(v))
}

// Area is where the layer goes, in the space's own coordinates. It is separate from
// drawing so that a hit test a frame later asks the same question.
func (o Overlay) Area(v grid.View) image.Rectangle {
	width, height := v.Size()
	return o.In(layout.Size{W: width, H: height})
}

// shade restyles every cell of the space it covers.
func (o Overlay) shade(v grid.View, width, height int) {
	for y := range height {
		for x := range width {
			if cell := v.CellAt(x, y); cell != nil {
				cell.Style = cell.Style.Merge(o.Shade)
			}
		}
	}
}
