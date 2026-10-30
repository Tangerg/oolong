package grid_test

import (
	"errors"
	"image"
	"io"
	"testing"

	"github.com/Tangerg/oolong/ui/core/grid"
)

type captureBlock struct {
	saved *grid.View
	paint bool
}

func (captureBlock) HeightForWidth(int) int { return 1 }
func (b captureBlock) Draw(v grid.View) {
	v.Text(0, 0, "中a", grid.Style{})
	if b.saved != nil {
		*b.saved = v
	}
	if b.paint {
		v.Paint(image.Rect(0, 0, 1, 1), 1, capturePainter{})
	}
}

type capturePainter struct{}

func (capturePainter) Paint(io.Writer, image.Point) error { return nil }
func (capturePainter) Erase(io.Writer) error              { return nil }

func TestCaptureReleasesDrawingAuthorityAndPreservesAtoms(t *testing.T) {
	var saved grid.View
	f, err := grid.Capture(grid.CaptureConfig{Content: captureBlock{saved: &saved}, Width: 4})
	if err != nil {
		t.Fatal(err)
	}
	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("capture view remained writable")
			}
		}()
		saved.Text(0, 0, "x", grid.Style{})
	}()
	rows, err := f.Rows(2)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0][0].Content() != "中" || rows[1][0].Content() != "a" {
		t.Fatalf("reflow=%+v", rows)
	}
	if _, err := f.Rows(1); !errors.Is(err, errors.ErrUnsupported) {
		t.Fatalf("narrow atom=%v", err)
	}
}

func TestCaptureRejectsNativePaintBeforePublication(t *testing.T) {
	if _, err := grid.Capture(grid.CaptureConfig{Content: captureBlock{paint: true}, Width: 4}); !errors.Is(err, errors.ErrUnsupported) {
		t.Fatalf("native paint=%v", err)
	}
}
