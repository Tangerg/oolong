package grid_test

import (
	"bytes"
	"errors"
	"io"
	"math"
	"testing"

	"github.com/Tangerg/oolong/ui/core/grid"
)

func TestDepthValidationCoversTheWholeDomain(t *testing.T) {
	for value := 0; value <= math.MaxUint8; value++ {
		depth := grid.Depth(value)
		err := depth.Validate()
		if depth <= grid.NoColor {
			if err != nil {
				t.Errorf("legal depth %d: %v", depth, err)
			}
		} else if !errors.Is(err, grid.ErrInvalidDepth) {
			t.Errorf("unknown depth %d: %v", depth, err)
		}
	}
}

func expectDepthPanic(t *testing.T, run func()) {
	t.Helper()
	defer func() {
		err, _ := recover().(error)
		if !errors.Is(err, grid.ErrInvalidDepth) {
			t.Errorf("invalid depth = %v", err)
		}
	}()
	run()
}

func TestInvalidDepthCannotChangeACanvasOrReachTheWire(t *testing.T) {
	for _, mode := range []string{"screen", "inline"} {
		t.Run(mode, func(t *testing.T) {
			var canvas interface {
				SetDepth(depth grid.Depth)
				Frame() grid.View
			}
			var flush func(io.Writer) error
			if mode == "screen" {
				screen := grid.NewScreen(2, 1)
				canvas, flush = screen, screen.Flush
			} else {
				inline := grid.NewInline(2, 1)
				canvas, flush = inline, func(out io.Writer) error { return inline.Flush(out) }
			}
			canvas.SetDepth(grid.Depth16)
			paint := func() { canvas.Frame().Text(0, 0, "x", grid.Style{FG: grid.RGBColor(12, 34, 56)}) }
			paint()
			var output bytes.Buffer
			if err := flush(&output); err != nil {
				t.Fatal(err)
			}
			output.Reset()
			expectDepthPanic(t, func() { canvas.SetDepth(grid.Depth(math.MaxUint8)) })
			paint()
			if err := flush(&output); err != nil || output.Len() != 0 {
				t.Fatalf("rejected depth changed the admitted baseline: bytes=%q err=%v", output.Bytes(), err)
			}
		})
	}
	expectDepthPanic(t, func() { _ = grid.EncodeRow(nil, grid.Depth(math.MaxUint8)) })
}
