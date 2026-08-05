package layout

import "image"

// Anchor is where a floating layer sits in the space it floats over.
type Anchor uint8

const (
	// Middle is the centre, which is where a modal belongs: it is the one position
	// that does not imply the thing it covers is still reachable.
	Middle Anchor = iota
	TopLeft
	Top
	TopRight
	Left
	Right
	BottomLeft
	Bottom
	BottomRight
)

// Placement is where a floating layer goes in the space it floats over.
//
// It is placement and nothing else — no drawing, no dimming, no idea what is going
// to be put there. Keeping it that way is what lets a hit test a frame later ask
// exactly the question the frame asked, and get exactly the same answer.
//
// The layer is clamped to the space rather than allowed to hang off the edge. A
// dialog whose buttons are past the right margin is a dialog nobody can answer.
type Placement struct {
	Anchor Anchor
	// Width and Height are the layer's size in cells. Zero means as large as the
	// space allows, less Margin.
	Width, Height int
	// Margin is kept clear between the layer and the edges of the space, so a layer
	// anchored to a corner does not look stuck to it.
	Margin int
}

// In is where the layer goes inside a space of the given size, in that space's own
// coordinates.
func (p Placement) In(space Size) image.Rectangle {
	room := image.Pt(max(space.W-2*p.Margin, 0), max(space.H-2*p.Margin, 0))
	size := image.Pt(p.Width, p.Height)
	if size.X <= 0 || size.X > room.X {
		size.X = room.X
	}
	if size.Y <= 0 || size.Y > room.Y {
		size.Y = room.Y
	}
	at := p.Anchor.place(room, size).Add(image.Pt(p.Margin, p.Margin))
	return image.Rect(at.X, at.Y, at.X+size.X, at.Y+size.Y)
}

// place works out where a box of size sits inside space.
func (a Anchor) place(space, size image.Point) image.Point {
	var at image.Point
	switch a {
	case TopLeft, Left, BottomLeft:
		at.X = 0
	case Top, Middle, Bottom:
		at.X = (space.X - size.X) / 2
	default:
		at.X = space.X - size.X
	}
	switch a {
	case TopLeft, Top, TopRight:
		at.Y = 0
	case Left, Middle, Right:
		at.Y = (space.Y - size.Y) / 2
	default:
		at.Y = space.Y - size.Y
	}
	return image.Pt(max(at.X, 0), max(at.Y, 0))
}
