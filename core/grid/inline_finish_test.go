package grid_test

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/Tangerg/oolong/core/grid"
)

func TestInlineFinishRefusesUnflushedOutput(t *testing.T) {
	for _, operation := range []string{"Print", "Append"} {
		for _, state := range []string{"new", "live", "finished"} {
			for _, width := range []int{0, 20} {
				t.Run(operation+"/"+state+"/"+map[int]string{0: "zero width", 20: "normal width"}[width], func(t *testing.T) {
					canvas := grid.NewInline(20, 2)
					if state != "new" {
						inline(t, canvas, grid.Cursor{}, lines("live"))
					}
					if state == "finished" {
						if err := canvas.Finish(io.Discard); err != nil {
							t.Fatal(err)
						}
					}
					draw := lines("accepted")
					var err error
					if operation == "Print" {
						err = canvas.Print(content(1, draw))
					} else {
						err = canvas.Append(func(view grid.View) bool { draw(view); return false })
					}
					if err != nil {
						t.Fatal(err)
					}
					canvas.Resize(width, 2)
					column, open := canvas.Tail()
					var output bytes.Buffer
					if width == 0 {
						if err := canvas.Flush(&output); err != nil || output.Len() != 0 {
							t.Fatalf("zero-width flush = %v, %q", err, output.String())
						}
					}
					if err := canvas.Finish(&output); err == nil || output.Len() != 0 {
						t.Fatalf("finish with unflushed output = %v, %q", err, output.String())
					}
					if after, stillOpen := canvas.Tail(); after != column || stillOpen != open {
						t.Fatalf("refused finish changed tail from (%d, %v) to (%d, %v)", column, open, after, stillOpen)
					}
					canvas.Resize(20, 2)
					canvas.Frame().Text(0, 0, "live", grid.Style{})
					if err := canvas.Flush(&output); err != nil {
						t.Fatal(err)
					}
					if !strings.Contains(output.String(), "accepted") {
						t.Fatalf("refused finish lost accepted output: %q", output.String())
					}
					output.Reset()
					if err := canvas.Finish(&output); err != nil || output.Len() == 0 {
						t.Fatalf("finish after flush = %v, %q", err, output.String())
					}
				})
			}
		}
	}
}
