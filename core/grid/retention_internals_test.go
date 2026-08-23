package grid

import (
	"image"
	"io"
	"strings"
	"testing"
	"unsafe"
)

type retainedPainter struct{ payload []byte }

func (*retainedPainter) Paint(io.Writer, image.Point) error { return nil }
func (*retainedPainter) Erase(io.Writer) error              { return nil }

func TestReusableFrameStorageReleasesDiscardedPayloads(t *testing.T) {
	surface := NewSurface(2, 2)
	surface.cells[3] = Cell{Content: "secret", Link: "https://example.test"}
	surface.Resize(1, 1)
	for i, cell := range surface.cells[:cap(surface.cells)] {
		if i >= len(surface.cells) && (cell.Content != "" || cell.Link != "") {
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
	inline.pending = append(inline.pending,
		printed{row: "one"}, printed{row: "two"}, printed{row: "three"},
	)
	if err := inline.Flush(io.Discard); err != nil {
		t.Fatal(err)
	}
	for i, pending := range inline.pending[:cap(inline.pending)] {
		if pending.row != "" {
			t.Fatalf("printed row %d retained transferred output", i)
		}
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
	for name, value := range map[string]string{"content": cell.Content, "link": cell.Link} {
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
