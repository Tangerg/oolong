package layout

import (
	"errors"
	"fmt"
	"image"
)

type Anchor uint8

const (
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

var ErrInvalidAnchor = errors.New("layout: invalid anchor")

func (a Anchor) Validate() error {
	if a > BottomRight {
		return fmt.Errorf("%w: %d", ErrInvalidAnchor, a)
	}
	return nil
}

// Placement is where a rectangle goes inside a larger space.
//
// It is geometry and has no knowledge of what occupies the returned rectangle. The
// result is clamped to the space rather than allowed to hang off an edge.
type Placement struct {
	Anchor Anchor
	// Width and Height are the rectangle's size. Zero means as large as the
	// space allows, less Margin.
	Width, Height int
	// Margin is kept clear between the rectangle and the edges of the space, so one
	// anchored to a corner remains separated from it.
	Margin int
}

// In panics for an invalid Anchor even without available space, so resizing cannot
// turn malformed placement into an apparently valid empty rectangle.
func (p Placement) In(space image.Point) image.Rectangle {
	if err := p.Anchor.Validate(); err != nil {
		panic(err)
	}
	space.X, space.Y = max(space.X, 0), max(space.Y, 0)
	margin := max(p.Margin, 0)
	room := image.Pt(Remaining(space.X, margin, margin), Remaining(space.Y, margin, margin))
	if room.X == 0 || room.Y == 0 {
		return image.Rectangle{}
	}
	size := image.Pt(p.Width, p.Height)
	if size.X <= 0 || size.X > room.X {
		size.X = room.X
	}
	if size.Y <= 0 || size.Y > room.Y {
		size.Y = room.Y
	}
	horizontal, vertical := p.Anchor.alignment()
	at := image.Pt(horizontal.Offset(room.X, size.X), vertical.Offset(room.Y, size.Y)).Add(image.Pt(margin, margin))
	return image.Rect(at.X, at.Y, at.X+size.X, at.Y+size.Y)
}

// alignment decomposes an anchor into the alignment it means on each axis, so
// placement reuses Align's one offset rule instead of a second copy of it.
func (a Anchor) alignment() (horizontal, vertical Align) {
	switch a {
	case TopLeft:
		return Start, Start
	case Top:
		return Center, Start
	case TopRight:
		return End, Start
	case Left:
		return Start, Center
	case Right:
		return End, Center
	case BottomLeft:
		return Start, End
	case Bottom:
		return Center, End
	case BottomRight:
		return End, End
	default:
		return Center, Center
	}
}
