package grid

import (
	"errors"
	"image"
	"io"
)

// Painter borrows terminal output for one synchronous callback. Both operations
// must preserve the cursor and terminal modes; Paint starts at the region's top-left.
// SGR and OSC 8 are isolated by the context. Native placements must use its Layer
// rather than inventing another stacking order. Native layers stay below text;
// registration order determines overlap within each plane. Erase removes persistent
// content; cell-only painters may leave erasure to the renderer's cell restoration.
type Painter interface {
	Paint(context PaintContext) error
	Erase(context PaintContext) error
}

// NativePainter binds a resource generation to its destination. Validation also
// runs before an unchanged region can be elided from a frame.
type NativePainter interface {
	Painter
	CheckTarget(target NativeTarget) error
	SamePlacement(other Painter) bool
}

type painted struct {
	rect image.Rectangle
	id   uint64
	by   Painter
}

// Zero deliberately requests repainting when a caller has no stable generation.
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

// NativeTarget separates physical destinations. Its zero value grants no
// host-bound resource access.
type NativeTarget struct{ id *byte }

// NewNativeTarget keeps resource capabilities from crossing terminal destinations.
func NewNativeTarget() NativeTarget { return NativeTarget{id: new(byte)} }

// Accepts confines native output to this destination's active callback; copied
// contexts lose access together when their callback returns.
func (t NativeTarget) Accepts(context PaintContext) bool {
	return context.scope != nil && context.scope.out != nil && t.id != nil && context.scope.target == t
}

// ErrForeignPaint prevents terminal-local wire IDs from naming resources on
// another destination.
var ErrForeignPaint = errors.New("grid: native resource belongs to another destination")

// PaintContext is a borrowed capability issued only by a renderer. Copies share
// its lifetime: writes after the callback returns fail with io.ErrClosedPipe.
// Size and Layer are immutable projections of the frame's composition.
type PaintContext struct{ scope *paintScope }

type paintScope struct {
	out    *[]byte
	target NativeTarget
	size   image.Point
	layer  int
}

func (c PaintContext) Write(p []byte) (int, error) {
	if c.scope == nil || c.scope.out == nil {
		return 0, io.ErrClosedPipe
	}
	*c.scope.out = append(*c.scope.out, p...)
	return len(p), nil
}

// Size bounds opaque output because the renderer cannot crop terminal-native
// content after it has been encoded. Dimensions are in cells, not pixels.
func (c PaintContext) Size() image.Point {
	if c.scope == nil {
		return image.Point{}
	}
	return c.scope.size
}

// Layer orders native content below text: later regions have larger negative
// layers. A protocol encoder must reject layers outside its representable range.
func (c PaintContext) Layer() int {
	if c.scope == nil {
		return 0
	}
	return c.scope.layer
}

func withPaintContext(target NativeTarget, out *[]byte, region painted, layer int, call func(PaintContext) error) error {
	scope := &paintScope{out: out, target: target, size: region.rect.Size(), layer: layer}
	// Opaque output can change rendition without updating any encoder cache.
	// Re-establish the baseline unconditionally at both sides of the lease.
	*out = append(*out, osc8Close...)
	*out = append(*out, sgrReset...)
	defer func() {
		scope.out = nil
		*out = append(*out, osc8Close...)
		*out = append(*out, sgrReset...)
	}()
	return call(PaintContext{scope: scope})
}

// Erasure may touch cells, so the renderer restores base cells only afterwards.
func eraseRegions(target NativeTarget, out *[]byte, regions []painted) error {
	for index, region := range regions {
		if err := withPaintContext(target, out, region, index-len(regions), region.by.Erase); err != nil {
			return err
		}
	}
	return nil
}

func paintRegions(target NativeTarget, out *[]byte, regions []painted, move func(image.Point)) error {
	for index, region := range regions {
		move(region.rect.Min)
		if err := withPaintContext(target, out, region, index-len(regions), region.by.Paint); err != nil {
			return err
		}
	}
	return nil
}

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
