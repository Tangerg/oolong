package grid

import (
	"image"
	"io"
)

// Painter is something that writes itself onto the terminal in a region of a frame,
// rather than into cells.
//
// It is what a cell cannot hold: a picture, a plot drawn in pixels, anything whose
// contents are bytes the terminal understands and this package does not. A frame
// keeps room for one with [View.Paint], writes the cells around it as usual, and
// then hands it the writer with the cursor already at the region's corner.
//
// # The one rule
//
// Paint must leave the cursor where it found it.
//
// That is not a nicety. A frame is written as a stream of movements from one known
// position to the next — an inline block's whole position is relative to where the
// last frame left the cursor — so a painter that moved it would move everything
// drawn after it. The rule is also, exactly, what makes a protocol usable in a
// region that redraws: the image protocol that can be told not to move the cursor is
// the same one that can be told to remove an image again, and the ones that cannot
// are the ones that only work in output that is never drawn over. See the graphics
// package, which says the same thing from the other side.
//
// # Erasing
//
// Some terminals remember what they were shown. An image placed by name stays until
// it is taken away, so a region that has gone — scrolled off, replaced, resized — has
// to be unsaid rather than merely painted over, and [Painter.Erase] is where that is
// written. A painter whose output is only cells has nothing to undo and writes
// nothing.
type Painter interface {
	// Paint writes what puts this in a region of cols by rows cells, with the
	// terminal's cursor already at its top-left corner, and leaves the cursor there.
	Paint(w io.Writer, cols, rows int) error
	// Erase writes what takes it off the terminal again, for a terminal that
	// remembers what it was shown, and leaves the cursor alone.
	Erase(w io.Writer) error
}

// painted is a region of a surface that something else writes into.
type painted struct {
	// rect is where it goes, in the surface's own coordinates.
	rect image.Rectangle
	// id says what is being painted, so a frame can tell whether a region still shows
	// the same thing as the frame before it.
	id uint64
	by Painter
}

// same reports whether two regions are showing the same thing in the same place,
// which is what lets an unchanged frame write nothing at all.
func (p painted) same(other painted) bool { return p.id == other.id && p.rect == other.rect }

// Paint keeps the region r of the frame for something that draws itself — see
// [Painter].
//
// The identity says what is being painted. Two frames that name the same thing in
// the same place write nothing between them, one that names something else replaces
// it, and one that names it nowhere takes it away; a caller with nothing to number
// by can pass zero, and then every frame is a different picture in the same place.
//
// A region that does not fit entirely inside what the view may draw on is not
// painted at all. Half a picture, squashed into the part that fits, is worse than
// none: this layer knows how many cells the region has and nothing about what is in
// it, so it cannot crop what it cannot read.
//
// The cells under it are left alone. What is painted goes behind them where the
// terminal allows it, which is what lets a caption be written over a picture.
func (v View) Paint(r image.Rectangle, id uint64, by Painter) {
	if v.surface == nil || by == nil || r.Empty() {
		return
	}
	area := translateRect(r, v.origin)
	if !area.In(v.clip) {
		return
	}
	v.surface.paints = append(v.surface.paints, painted{rect: area, id: id, by: by})
}

// regions is what the surface's frame keeps room for.
func (s *Surface) regions() []painted {
	if s == nil {
		return nil
	}
	return s.paints
}

// bytesTo lets a painter write into a frame's own buffer, so one frame is still one
// write to the terminal.
type bytesTo struct{ buf *[]byte }

func (b bytesTo) Write(p []byte) (int, error) {
	*b.buf = append(*b.buf, p...)
	return len(p), nil
}

// repaint writes what turns the regions the terminal is showing into the regions
// this frame asked for, positioning each with move.
//
// Everything that changed is erased before anything is painted. A region that moved
// is the same picture in two places to a terminal that remembers it, and painting
// the new one first would leave both on screen until the old one was taken away —
// which is the one order that flickers.
func repaint(out io.Writer, was, now []painted, move func(image.Point)) error {
	if len(was) == 0 && len(now) == 0 {
		return nil
	}
	for _, before := range was {
		if held(now, before) {
			continue
		}
		if err := before.by.Erase(out); err != nil {
			return err
		}
	}
	for _, region := range now {
		if held(was, region) {
			continue
		}
		move(region.rect.Min)
		if err := region.by.Paint(out, region.rect.Dx(), region.rect.Dy()); err != nil {
			return err
		}
	}
	return nil
}

// held reports whether a region is in a set unchanged.
func held(set []painted, region painted) bool {
	for _, other := range set {
		if other.same(region) {
			return true
		}
	}
	return false
}

// sameRegions reports whether repaint would leave every region alone. Regions are
// a set for this purpose: drawing the same identity twice does not make two terminal
// objects, and order does not change what is on screen.
func sameRegions(a, b []painted) bool {
	for _, region := range a {
		if !held(b, region) {
			return false
		}
	}
	for _, region := range b {
		if !held(a, region) {
			return false
		}
	}
	return true
}
