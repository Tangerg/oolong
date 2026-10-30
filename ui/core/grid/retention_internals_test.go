package grid

import (
	"errors"
	"image"
	"io"
	"strings"
	"testing"
	"unsafe"
)

type scratchContent struct {
	result   string
	retained *View
}

func (scratchContent) HeightForWidth(int) int { return 1 }

func (c scratchContent) Draw(view View) {
	if c.retained != nil {
		*c.retained = view
	}
	view.Text(0, 0, "private", Style{})
	view.Link(0, 0, 7, "https://example.test/private")
	if c.result != "accepted" {
		view.Paint(Area(0, 0, 1, 1), 1, &retainedPainter{payload: []byte("private pixels")})
	}
	if c.result == "panic" {
		panic("drawing interrupted")
	}
}

func TestCaptureReleasesItsTemporaryDrawing(t *testing.T) {
	for _, result := range []string{"accepted", "refused", "panic"} {
		t.Run(result, func(t *testing.T) {
			var retained View
			var err error
			var panicked any
			func() {
				defer func() { panicked = recover() }()
				_, err = Capture(CaptureConfig{Width: 20, Content: scratchContent{result: result, retained: &retained}})
			}()
			if result == "refused" && !errors.Is(err, errors.ErrUnsupported) || result == "accepted" && err != nil {
				t.Fatalf("capture=%v", err)
			}
			if (panicked != nil) != (result == "panic") {
				t.Fatalf("panic=%v", panicked)
			}
			for _, cell := range retained.surface.cells {
				if cell != (Cell{}) {
					t.Fatal("expired capture retained temporary payload")
				}
			}
			for _, region := range retained.surface.paints[:cap(retained.surface.paints)] {
				if region.by != nil {
					t.Fatal("expired capture retained native painter")
				}
			}
		})
	}
}

type retainedPainter struct{ payload []byte }

func (*retainedPainter) Paint(io.Writer, image.Point) error { return nil }
func (*retainedPainter) Erase(io.Writer) error              { return nil }

func TestReusableFrameStorageReleasesDiscardedPayloads(t *testing.T) {
	surface := NewSurface(2, 2)
	surface.cells[3] = Cell{content: "secret", Link: "https://example.test"}
	surface.Resize(1, 1)
	for i, cell := range surface.cells[:cap(surface.cells)] {
		if i >= len(surface.cells) && (cell.content != "" || cell.Link != "") {
			t.Fatalf("cell %d retained discarded content %+v", i, cell)
		}
	}

	painter := &retainedPainter{payload: []byte("pixels")}
	surface.paints = append(surface.paints,
		painted{by: painter}, painted{by: painter}, painted{by: painter},
	)
	surface.Reset()
	for i, region := range surface.paints[:cap(surface.paints)] {
		if region.by != nil {
			t.Fatalf("paint region %d retained a discarded painter", i)
		}
	}

	inline := NewInline(2, 1)
	frozen, err := Capture(CaptureConfig{Width: 2, Content: scratchContent{result: "accepted"}})
	if err != nil {
		t.Fatal(err)
	}
	candidate, err := inline.Prepare(frozen)
	if err != nil {
		t.Fatal(err)
	}
	if inline.contribution != nil {
		t.Fatal("preparation retained temporary contribution rows")
	}
	candidate.Commit()
	if candidate.payload != nil || candidate.settle != nil {
		t.Fatal("settled output retains payload or canvas callback")
	}
}

func TestCellsOwnTextAndLinksAtTheDrawingBoundary(t *testing.T) {
	source := strings.Repeat("discarded ", 1<<12) + "kept destination"
	textAt := strings.Index(source, "kept")
	linkAt := strings.Index(source, "destination")
	surface := NewSurface(4, 1)
	view := surface.View()
	view.Text(0, 0, source[textAt:textAt+len("kept")], Style{})
	view.Link(0, 0, 4, source[linkAt:])

	sourceStart := uintptr(unsafe.Pointer(unsafe.StringData(source))) //nolint:gosec // Test compares allocation identity and never dereferences the address.
	sourceEnd := sourceStart + uintptr(len(source))
	cell, _ := surface.CellAt(0, 0)
	for name, value := range map[string]string{"content": cell.Content(), "link": cell.Link} {
		at := uintptr(unsafe.Pointer(unsafe.StringData(value))) //nolint:gosec // Test compares allocation identity and never dereferences the address.
		if at >= sourceStart && at < sourceEnd {
			t.Errorf("cell %s still shares the caller's source allocation", name)
		}
	}
}

func TestLinkWorkIsBoundedByTheVisibleSurface(t *testing.T) {
	maxInt := int(^uint(0) >> 1)
	surface := NewSurface(2, 1)
	surface.View().Link(0, 0, maxInt, "target")
	for x := range 2 {
		cell, _ := surface.CellAt(x, 0)
		if cell.Link != "target" {
			t.Fatalf("cell %d link = %q, want target", x, cell.Link)
		}
	}
}
