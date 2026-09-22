package grid_test

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/Tangerg/oolong/core/grid"
)

func TestResizeKeepsPublishedImageCleanup(t *testing.T) {
	for name, canvas := range map[string]interface {
		Frame() grid.View
		Resize(cols, rows int)
		Flush(w io.Writer) error
	}{"screen": grid.NewScreen(10, 4), "inline": grid.NewInline(10, 4)} {
		t.Run(name, func(t *testing.T) {
			var log []string
			canvas.Frame().Paint(grid.Rect(1, 1, 2, 1), 1, picture{name: "old", log: &log})
			var out bytes.Buffer
			if err := canvas.Flush(&out); err != nil {
				t.Fatal(err)
			}
			log = nil
			canvas.Resize(12, 5)
			canvas.Frame()
			if err := canvas.Flush(&out); err != nil {
				t.Fatal(err)
			}
			if strings.Join(log, ",") != "erase old" {
				t.Fatalf("lost cleanup: %v", log)
			}
		})
	}
}

func TestTextAndLinksRejectRawControlBytesWithoutCorruptingUTF8(t *testing.T) {
	for _, source := range []string{"\x9b31m", "中"} {
		screen := grid.NewScreen(20, 1)
		view := screen.Frame()
		view.Text(0, 0, source, grid.Style{})
		view.Link(0, 0, 3, "https://example.test/"+source)
		var out bytes.Buffer
		if err := screen.Flush(&out); err != nil {
			t.Fatal(err)
		}
		if source[0] == 0x9b && bytes.Contains(out.Bytes(), []byte{0x9b}) {
			t.Fatal("raw C1 escaped trust boundary")
		}
		if source == "中" && !strings.Contains(out.String(), "中") {
			t.Fatal("valid UTF-8 was filtered")
		}
	}
}

func TestCopyRowsPreservesOverlappingSource(t *testing.T) {
	s := grid.NewSurface(1, 4)
	for y, r := range "ABCD" {
		s.View().Text(0, y, string(r), grid.Style{})
	}
	s.CopyRows(s, 0, 1, 3)
	if strings.Join(s.Rows(), "") != "AABC" {
		t.Fatal(s.Rows())
	}
}

func TestImageOnlyChangeRestoresTheVisibleCursor(t *testing.T) {
	screen := grid.NewScreen(20, 5)
	var log []string
	var out bytes.Buffer
	for id := uint64(1); id <= 2; id++ {
		view := screen.Frame()
		view.Paint(grid.Rect(0, 0, 2, 1), id, picture{name: "image", log: &log})
		view.PlaceCursor(9, 3, grid.CursorStyle{})
		out.Reset()
		if err := screen.Flush(&out); err != nil {
			t.Fatal(err)
		}
	}
	if !strings.Contains(out.String(), "\x1b[4;10H") {
		t.Fatalf("cursor not restored: %q", out.String())
	}
}

func TestCellScrollCannotMoveAnUnchangedImagePlacement(t *testing.T) {
	screen := grid.NewScreen(24, 10)
	rows := []string{"alpha", "bravo", "charlie", "delta", "echo", "foxtrot", "golf", "hotel", "india", "juliett", "kilo"}
	var log []string
	var out bytes.Buffer
	for shift := range 2 {
		view := screen.Frame()
		for y := range 10 {
			view.Text(0, y, rows[y+shift], grid.Style{})
		}
		view.Paint(grid.Rect(20, 3, 2, 1), 1, picture{name: "fixed", log: &log})
		out.Reset()
		if err := screen.Flush(&out); err != nil {
			t.Fatal(err)
		}
	}
	if strings.Contains(out.String(), "\x1b[1S") {
		t.Fatalf("hardware scroll moved a fixed image: %q", out.String())
	}
}
