package grid_test

import (
	"errors"
	"image"
	"strings"
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

func (capturePainter) Paint(grid.PaintContext) error { return nil }
func (capturePainter) Erase(grid.PaintContext) error { return nil }

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

func TestCaptureBudgetsTheOwnedDestinationOnce(t *testing.T) {
	const width = 256
	target := strings.Repeat("target", 512)
	block := content(1, func(view grid.View) {
		view.Text(0, 0, strings.Repeat("x", width), grid.Style{})
		view.Link(0, 0, width, target)
	})
	f, err := grid.Capture(grid.CaptureConfig{Width: width, Content: block, MaxBytes: 64 << 10})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := f.Rows(width)
	if err != nil || len(rows) != 1 || len(rows[0]) != width {
		t.Fatal("unexpected projection", err)
	}
	for _, cell := range rows[0] {
		if cell.Link != target {
			t.Fatal("lost hyperlink from frozen row")
		}
	}
}

func TestCaptureRefusesOversizedStringBeforeCopyingIt(t *testing.T) {
	target := strings.Repeat("a", 1<<16)
	b := content(1, func(v grid.View) {
		v.Link(0, 0, 1, target)
		if c, _ := v.CellAt(0, 0); c.Link != "" {
			t.Fatal("over-budget address was installed during drawing")
		}
	})
	if _, err := grid.Capture(grid.CaptureConfig{Content: b, Width: 1, MaxBytes: 1024}); !errors.Is(err, grid.ErrCaptureLimit) {
		t.Fatalf("oversized link: %v", err)
	}
	var typedNil *captureBlock
	if f, err := grid.Capture(grid.CaptureConfig{Content: typedNil, Width: 1}); err != nil || f.RowCount() != 0 {
		t.Fatalf("nil body produced content: %v, %v", f, err)
	}
}
