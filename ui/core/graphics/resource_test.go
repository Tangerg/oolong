package graphics_test

import (
	"bytes"
	"errors"
	"image"
	"io"
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
			if err := foreign.Placement(1).Erase(&out); !errors.Is(err, grid.ErrForeignPaint) || out.Len() != 0 {
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
