package grid_test

import (
	"bytes"
	"errors"
	"image"
	"io"
	"math"
	"testing"

	"github.com/Tangerg/oolong/ui/core/grid"
)

func expectCursorShapePanic(t *testing.T, run func()) {
	t.Helper()
	defer func() {
		err, _ := recover().(error)
		if !errors.Is(err, grid.ErrInvalidCursorShape) {
			t.Errorf("invalid cursor shape = %v", err)
		}
	}()
	run()
}

func TestInvalidCursorShapeCannotChangeACanvasOrReachTheWire(t *testing.T) {
	invalid := grid.CursorStyle{Shape: grid.CursorShape(math.MaxUint8), Blink: true}
	for _, mode := range []string{"screen", "inline"} {
		t.Run(mode, func(t *testing.T) {
			var canvas interface {
				Frame() grid.View
				Cursor() grid.Cursor
			}
			var flush func(io.Writer) error
			if mode == "screen" {
				screen := grid.NewScreen(2, 1)
				canvas, flush = screen, screen.Flush
			} else {
				inline := grid.NewInline(2, 1)
				canvas, flush = inline, func(out io.Writer) error { return inline.Flush(out) }
			}
			style := grid.CursorStyle{Shape: grid.CursorBar, Blink: true}
			canvas.Frame().PlaceCursor(1, 0, style)
			var output bytes.Buffer
			if err := flush(&output); err != nil {
				t.Fatal(err)
			}
			output.Reset()
			view := canvas.Frame()
			view.PlaceCursor(1, 0, style)
			before := canvas.Cursor()
			for _, at := range []image.Point{image.Pt(0, 0), image.Pt(2, 1)} {
				expectCursorShapePanic(t, func() { view.PlaceCursor(at.X, at.Y, invalid) })
				if got := canvas.Cursor(); got != before {
					t.Fatalf("rejected cursor changed the frame: got %+v, want %+v", got, before)
				}
			}
			if err := flush(&output); err != nil || output.Len() != 0 {
				t.Fatalf("rejected cursor changed the wire baseline: bytes=%q err=%v", output.Bytes(), err)
			}
		})
	}
	for _, view := range []grid.View{{}, grid.NewSurface(2, 1).View()} {
		expectCursorShapePanic(t, func() { view.PlaceCursor(0, 0, invalid) })
	}
}
