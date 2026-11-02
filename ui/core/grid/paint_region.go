package grid

import (
	"errors"
	"image"
	"io"
)

// Painter writes terminal-native content into a frame region. Paint must preserve
// the cursor or subsequent relative output moves to the wrong position. Erase removes
// persistent content before the next composition restores its cells and paints its
// regions. Cell-only painters may leave erasure to that restoration. The writer
// belongs to the synchronous callback; after it returns, writes fail with [io.ErrClosedPipe].
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

// Erasure can touch cells as well as named resources. Renderers must restore their
// base cells after this step and only then draw the next ordered composition.
func eraseRegions(target NativeTarget, out *[]byte, regions []painted) error {
	for _, region := range regions {
		if err := withPaintWriter(target, out, region.by.Erase); err != nil {
			return err
		}
	}
	return nil
}

func paintRegions(target NativeTarget, out *[]byte, regions []painted, move func(image.Point)) error {
	for _, region := range regions {
		move(region.rect.Min)
		if err := withPaintWriter(target, out, func(w io.Writer) error { return region.by.Paint(w, region.rect.Size()) }); err != nil {
			return err
		}
	}
	return nil
}

// Drawing order is part of the composition even when every identity and
// rectangle survives; generic painters need not have independent native layers.
func sameRegions(a, b []painted) bool {
	if len(a) != len(b) {
		return false
	}
	for i, region := range a {
		if !region.same(b[i]) {
			return false
		}
	}
	return true
}
