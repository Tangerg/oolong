package graphics_test

import (
	"bytes"
	"errors"
	"image"
	imagepng "image/png"
	"io"
	"strings"
	"testing"

	"github.com/Tangerg/oolong/ui/core/graphics"
	"github.com/Tangerg/oolong/ui/core/grid"
)

func TestResourcePlacementUsesDestinationAndTransmissionGeneration(t *testing.T) {
	for _, mode := range []string{"screen", "inline"} {
		t.Run(mode, func(t *testing.T) {
			var owner, other graphics.ImageOwner
			target := owner.NativeTarget()
			var frame func() grid.View
			var flush func(io.Writer) error
			if mode == "screen" {
				canvas := target.Screen(5, 2)
				frame, flush = canvas.Frame, canvas.Flush
			} else {
				canvas := target.Inline(5, 2)
				frame = canvas.Frame
				flush = func(w io.Writer) error { return canvas.Flush(w) }
			}
			wire := graphics.Image{ID: 1, Size: image.Pt(3, 2)}
			local, foreign := owner.Bind(wire), other.Bind(wire)
			var out bytes.Buffer
			draw := func(r graphics.Resource) error {
				out.Reset()
				frame().Paint(grid.Area(0, 0, 2, 1), 1, r.Placement(1))
				return flush(&out)
			}
			if err := draw(local); err != nil || !bytes.Contains(out.Bytes(), []byte("a=p,i=1,p=1")) {
				t.Fatalf("local placement = %q, %v", out.Bytes(), err)
			}
			if err := draw(foreign); !errors.Is(err, grid.ErrForeignPaint) || out.Len() != 0 {
				t.Fatalf("foreign placement reusing a visible ID = %q, %v", out.Bytes(), err)
			}
			if err := foreign.Placement(1).Erase(grid.PaintContext{}); !errors.Is(err, grid.ErrForeignPaint) || out.Len() != 0 {
				t.Fatalf("unbound erase = %q, %v", out.Bytes(), err)
			}
			deny := errors.New("admission paused")
			if err := owner.Release(local, func(graphics.Image) error { return deny }); !errors.Is(err, deny) || !local.Live() {
				t.Fatalf("refused release lost resource: %v", err)
			}
			if err := owner.Release(local, func(graphics.Image) error { return nil }); err != nil || local.Live() {
				t.Fatalf("accepted release: %v", err)
			}
			if err := draw(local); !errors.Is(err, graphics.ErrImageReleased) || out.Len() != 0 {
				t.Fatalf("retired placement elided as unchanged = %q, %v", out.Bytes(), err)
			}
			next := owner.Bind(wire)
			if err := draw(next); err != nil || !bytes.Contains(out.Bytes(), []byte("a=p,i=1,p=1")) || bytes.Contains(out.Bytes(), []byte("a=d")) {
				t.Fatalf("new transmission same wire ID = %q, %v", out.Bytes(), err)
			}
			if err := owner.Release(local, func(graphics.Image) error { t.Fatal("stale release sent a delete"); return nil }); !errors.Is(err, graphics.ErrImageReleased) || !next.Live() {
				t.Fatalf("stale release disturbed replacement: %v", err)
			}
			owner.RevokeAll()
			if next.Live() || !foreign.Live() {
				t.Fatal("revocation crossed the issuing owner")
			}
		})
	}
}

func TestRevocationCannotReassignAWireIDInsideItsDelete(t *testing.T) {
	var owner graphics.ImageOwner
	wire := graphics.Image{ID: 1}
	first := owner.Bind(wire)
	refusal := errors.New("transport refused")
	err := owner.Release(first, func(graphics.Image) error {
		owner.RevokeAll()
		if first.Live() {
			t.Fatal("revoked transmission stayed live")
		}
		func() {
			defer func() {
				if recover() == nil {
					t.Error("new transmission reused the in-flight delete's ID")
				}
			}()
			owner.Bind(wire)
		}()
		return refusal
	})
	if !errors.Is(err, refusal) || first.Live() || !owner.Bind(wire).Live() {
		t.Fatal("delete settlement revived a revoked resource or stranded its wire ID")
	}
}

func TestCropsShareOneTransmissionAndKeepIndependentPlacements(t *testing.T) {
	var data, out bytes.Buffer
	if err := imagepng.Encode(&data, image.NewGray(image.Rect(0, 0, 80, 40))); err != nil {
		t.Fatal(err)
	}
	wire, err := graphics.Transmit(&out, 31, data.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	var owner graphics.ImageOwner
	resource := owner.Bind(wire)
	left, right := resource.Placement(1), resource.Placement(2)
	left.Source = image.Rect(-5, 10, 20, 50)
	right.Source = image.Rect(20, 10, 40, 40)
	if got := left.SourceBounds(); got != image.Rect(0, 10, 20, 40) {
		t.Fatalf("source bounds = %v", got)
	}
	canvas := owner.NativeTarget().Screen(4, 3)
	draw := func() string {
		t.Helper()
		out.Reset()
		frame := canvas.Frame()
		left.Draw(frame, image.Rect(0, 0, 2, 3))
		right.Draw(frame, image.Rect(2, 0, 4, 3))
		if err := canvas.Flush(&out); err != nil {
			t.Fatal(err)
		}
		return out.String()
	}
	first := draw()
	if !strings.Contains(first, "a=p,i=31,p=1,x=0,y=10,w=20,h=30,c=2,r=3,") ||
		!strings.Contains(first, "a=p,i=31,p=2,x=20,y=10,w=20,h=30,c=2,r=3,") ||
		strings.Contains(first, "a=t") {
		t.Fatalf("placing two crops: %q", first)
	}
	if unchanged := draw(); strings.Contains(unchanged, "a=p") {
		t.Fatalf("unchanged crops were repainted: %q", unchanged)
	}
	left.Source = image.Rect(40, 10, 60, 40)
	// A changed opaque composition is rebuilt in drawing order. Each crop must
	// retain its own source and placement while sharing the transmitted image.
	if next := draw(); !strings.Contains(next, "a=p,i=31,p=1,x=40,y=10,w=20,h=30,c=2,r=3,") ||
		!strings.Contains(next, "a=p,i=31,p=2,x=20,y=10,w=20,h=30,c=2,r=3,") ||
		strings.Contains(next, "a=t") {
		t.Fatalf("updating crops lost placement geometry or retransmitted data: %q", next)
	}
	if err := owner.Release(resource, func(i graphics.Image) error { return i.Release(&out) }); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	left.Draw(canvas.Frame(), image.Rect(0, 0, 2, 3))
	if err := canvas.Flush(&out); !errors.Is(err, graphics.ErrImageReleased) || out.Len() != 0 {
		t.Fatalf("cropped placement after release = %q, %v", &out, err)
	}
}

func TestPlacementRejectsAnEmptySourceIntersectionBeforeWriting(t *testing.T) {
	var out bytes.Buffer
	p := (graphics.Image{ID: 3, Size: image.Pt(12, 12)}).Placement(1)
	for _, crop := range []image.Rectangle{image.Rect(1, 2, 1, 7), image.Rect(12, 0, 24, 10)} {
		p.Source = crop
		if err := p.Paint(&out, image.Pt(2, 2), -1); err == nil || out.Len() != 0 {
			t.Fatalf("painting %v wrote %q, err = %v", crop, &out, err)
		}
	}
}
