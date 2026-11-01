package grid

import (
	"errors"
	"image"
	"io"
)

// Painter writes terminal-native content into a frame region. Paint must preserve
// the cursor or subsequent relative output moves to the wrong position. Erase removes
// persistent named images when their region disappears; cell-only painters need no
// erasure output. The writer belongs to the synchronous callback; after it returns,
// writes fail with [io.ErrClosedPipe].
type Painter interface {
	// Paint writes what puts this in a region of size cells, with the
	// terminal's cursor already at its top-left corner, and leaves the cursor there.
	Paint(w io.Writer, size image.Point) error
	// Erase writes what takes it off the terminal again, for a terminal that
	// remembers what it was shown, and leaves the cursor alone.
	Erase(w io.Writer) error
}

// NativePainter is a Painter bound to a destination and a particular resource
// generation. Validation also runs for unchanged regions, before a diff may
// elide painting. SamePlacement compares two native resources after their
// caller-supplied region ID and rectangle already match.
type NativePainter interface {
	Painter
	CheckTarget(target NativeTarget) error
	SamePlacement(other Painter) bool
}

// painted is a region of a surface that something else writes into.
type painted struct {
	rect image.Rectangle
	// id says what is being painted, so a frame can tell whether a region still shows
	// the same thing as the frame before it.
	id uint64
	by Painter
}

// same reports whether two regions are showing the same thing in the same place,
// which is what lets an unchanged frame write nothing at all.
// Zero is deliberately not an identity: it is the caller's way to request a fresh
// paint when it has no stable generation of its own.
func (p painted) same(other painted) bool {
	if p.id == 0 || p.id != other.id || p.rect != other.rect {
		return false
	}
	if n, ok := p.by.(NativePainter); ok {
		return n.SamePlacement(other.by)
	}
	_, native := other.by.(NativePainter)
	return !native
}

// NativeTarget is the identity of a native drawing destination. Its zero value
// accepts ordinary painters, but grants no host-bound resource access.
type NativeTarget struct{ id *byte }

// NewNativeTarget allocates a distinct physical destination identity.
func NewNativeTarget() NativeTarget { return NativeTarget{id: new(byte)} }

// Accepts reports whether w is this destination's live painter callback.
func (t NativeTarget) Accepts(w io.Writer) bool {
	p, ok := w.(*paintWriter)
	return ok && p.buf != nil && t.id != nil && p.target == t
}

// ErrForeignPaint rejects a region belonging to a different native destination.
var ErrForeignPaint = errors.New("grid: native resource belongs to another destination")

// Each painter borrows the encoder only for its own callback.
type paintWriter struct {
	buf    *[]byte
	target NativeTarget
}

func (w *paintWriter) Write(p []byte) (int, error) {
	if w.buf == nil {
		return 0, io.ErrClosedPipe
	}
	*w.buf = append(*w.buf, p...)
	return len(p), nil
}

func withPaintWriter(target NativeTarget, buf *[]byte, call func(io.Writer) error) error {
	w := &paintWriter{buf: buf, target: target}
	defer func() { w.buf = nil }()
	return call(w)
}

// repaint writes what turns the regions the terminal is showing into the regions
// this frame asked for, positioning each with move.
//
// Everything that changed is erased before anything is painted. A region that moved
// is the same picture in two places to a terminal that remembers it, and painting
// the new one first would leave both on screen until the old one was taken away —
// which is the one order that flickers.
func repaint(target NativeTarget, out *[]byte, was, now []painted, move func(image.Point)) error {
	if len(was) == 0 && len(now) == 0 {
		return nil
	}
	for _, before := range was {
		if held(now, before) {
			continue
		}
		if err := withPaintWriter(target, out, before.by.Erase); err != nil {
			return err
		}
	}
	for _, region := range now {
		if held(was, region) {
			continue
		}
		move(region.rect.Min)
		if err := withPaintWriter(target, out, func(w io.Writer) error { return region.by.Paint(w, region.rect.Size()) }); err != nil {
			return err
		}
	}
	return nil
}

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
