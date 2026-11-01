package grid_test

import (
	"bytes"
	"errors"
	"image"
	"io"
	"testing"

	"github.com/Tangerg/oolong/ui/core/grid"
)

func TestRetainedSurfaceViewClipsToCurrentStorage(t *testing.T) {
	s := grid.NewSurface(5, 3)
	v := s.View().Sub(grid.Area(1, 0, 3, 2))
	right := v.Sub(grid.Area(2, 0, 1, 1))
	s.Resize(2, 1)
	if got := v.Visible(); got != grid.Area(0, 0, 1, 1) || !right.Empty() || right.MergeStyle(0, 0, grid.Style{}) {
		t.Errorf("stale visibility: visible=%v, right empty=%t", got, right.Empty())
	}
	v.Fill(v.Bounds(), grid.Style{})
	if v.Text(0, 0, "AB", grid.Style{}) != 2 || s.Rows()[0] != " A" {
		t.Fatal("clipped drawing lost its visible prefix or logical width")
	}
}

type writerBorrow struct{ onPaint, onErase func(io.Writer) }

func (b writerBorrow) Paint(w io.Writer, _ image.Point) error {
	if b.onPaint != nil {
		b.onPaint(w)
	}
	return nil
}

func (b writerBorrow) Erase(w io.Writer) error {
	if b.onErase != nil {
		b.onErase(w)
	}
	return nil
}

func TestNativePainterCannotWriteIntoAnotherCallback(t *testing.T) {
	for _, canvas := range []regionCanvas{grid.NewScreen(4, 2), grid.NewInline(4, 2)} {
		var painted, erased io.Writer
		old := writerBorrow{onPaint: func(w io.Writer) { painted = w }, onErase: func(w io.Writer) { erased = w }}
		canvas.Frame().Paint(grid.Area(0, 0, 1, 1), 1, old)
		if err := flushCanvas(canvas, io.Discard); err != nil {
			t.Fatal(err)
		}
		var buf bytes.Buffer
		canvas.Frame().Paint(grid.Area(1, 0, 1, 1), 2, writerBorrow{onPaint: func(w io.Writer) {
			for _, expired := range []io.Writer{painted, erased} {
				if n, err := io.WriteString(expired, "stale region"); n != 0 || !errors.Is(err, io.ErrClosedPipe) {
					t.Errorf("expired painter wrote %d: %v", n, err)
				}
			}
			_, _ = io.WriteString(w, "live region")
		}})
		if err := flushCanvas(canvas, &buf); err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(buf.Bytes(), []byte("stale region")) || !bytes.Contains(buf.Bytes(), []byte("live region")) {
			t.Errorf("bad frame: %q", buf.String())
		}
	}
}

func TestOutputAliasesShareSettlement(t *testing.T) {
	screen := grid.NewScreen(3, 1)
	screen.Frame().Text(0, 0, "x", grid.Style{})
	first, err := screen.Prepare()
	if err != nil {
		t.Fatal(err)
	}
	alias := *first
	first.Commit()
	screen.Frame().Text(0, 0, "y", grid.Style{})
	second, err := screen.Prepare()
	if err != nil {
		t.Fatal(err)
	}
	defer second.Abort()
	if alias.Bytes() != nil || alias.Len() != 0 {
		t.Fatal("settled alias retained an admitted payload")
	}
	alias.Abort()
	rejectsPublicationReentry(t, func() { screen.Frame() })
}
