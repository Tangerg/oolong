package grid

import (
	"io"
	"testing"
)

type retainedPainter struct{ payload []byte }

func (*retainedPainter) Paint(io.Writer, int, int) error { return nil }
func (*retainedPainter) Erase(io.Writer) error           { return nil }

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
