package grid_test

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/Tangerg/oolong/ui/core/grid"
	uitext "github.com/Tangerg/oolong/ui/core/text"
)

type styledRegion struct{ paint, erase string }

func (r styledRegion) Paint(context grid.PaintContext) error {
	_, err := io.WriteString(context, r.paint)
	return err
}

func (r styledRegion) Erase(context grid.PaintContext) error {
	_, err := io.WriteString(context, r.erase)
	return err
}

func TestPainterRenditionIsConfinedToItsCallback(t *testing.T) {
	const styled = "\x1b[1m\x1b]8;;https://example.test/region\x1b\\"
	for name, canvas := range map[string]regionCanvas{
		"screen": grid.NewScreen(6, 2),
		"inline": grid.NewInline(6, 2),
	} {
		t.Run(name, func(t *testing.T) {
			frame := canvas.Frame()
			frame.Paint(grid.Area(0, 0, 1, 1), 1, styledRegion{paint: styled + "A\x1b[D", erase: styled})
			frame.Paint(grid.Area(2, 0, 1, 1), 2, styledRegion{paint: "B\x1b[D"})
			var output bytes.Buffer
			if err := flushCanvas(canvas, &output); err != nil {
				t.Fatal(err)
			}
			assertPlainOutput(t, output.String()+"Z", "BZ")

			output.Reset()
			frame = canvas.Frame()
			frame.Text(0, 0, "C", grid.Style{})
			frame.Paint(grid.Area(2, 0, 1, 1), 2, styledRegion{paint: "B\x1b[D"})
			if err := flushCanvas(canvas, &output); err != nil {
				t.Fatal(err)
			}
			assertPlainOutput(t, output.String()+"Z", "CBZ")
		})
	}
}

func assertPlainOutput(t *testing.T, output, plain string) {
	t.Helper()
	var painted strings.Builder
	for _, line := range uitext.Decode(output, grid.Style{}) {
		for _, span := range line {
			painted.WriteString(span.Text)
			if strings.ContainsAny(span.Text, plain) && (span.Style != (grid.Style{}) || span.Link != "") {
				t.Errorf("rendition leaked into %#v in %q", span, output)
			}
		}
	}
	for _, r := range plain {
		if !strings.ContainsRune(painted.String(), r) {
			t.Errorf("missing expected character %q in %q", r, output)
		}
	}
}
