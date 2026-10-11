package kit

import (
	"image"

	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/layout"
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
	Theme Theme
}

// Draw shades what is behind the layer and returns the view to draw the layer into.
func (o Overlay) Draw(v grid.View) grid.View {
	width, height := v.Size()
	if width <= 0 || height <= 0 {
		return v.Sub(image.Rectangle{})
	}
	// Mixed rather than filled: what is behind stays legible and simply recedes,
	// which is what tells the reader it is still there and not gone.
	o.Theme.Scrim.Over(v)
	return v.Sub(o.Area(v))
}

// Area is where the layer goes, in the space's own coordinates. It is separate from
// drawing so that a hit test a frame later asks the same question.
func (o Overlay) Area(v grid.View) image.Rectangle {
	return o.In(v.Bounds().Size())
}
