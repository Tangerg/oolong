package grid_test

import (
	"bytes"
	"fmt"
	"image"
	"io"
	"strings"
	"testing"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/uitest/ptytest"
)

func TestPaintSequenceOwnsOverlappingComposition(t *testing.T) {
	type region struct {
		id   uint64
		name string
		area image.Rectangle
	}
	background := region{1, "A", grid.Area(1, 0, 3, 1)}
	foreground := region{2, "B", grid.Area(2, 0, 1, 1)}
	moved := region{1, "A", grid.Area(2, 0, 3, 1)}
	for _, test := range []struct {
		name   string
		before []region
		after  []region
		want   string
	}{
		{
			name:   "reorder",
			before: []region{background, foreground}, after: []region{foreground, background},
			want: ".AAA....",
		},
		{
			name:   "insert beneath retained foreground",
			before: []region{foreground}, after: []region{background, foreground},
			want: ".ABA....",
		},
		{
			name:   "remove foreground",
			before: []region{background, foreground}, after: []region{background},
			want: ".AAA....",
		},
		{
			name:   "move beneath retained foreground",
			before: []region{background, foreground}, after: []region{moved, foreground},
			want: "..BAA...",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			for name, makeCanvas := range map[string]func() regionCanvas{
				"screen": func() regionCanvas { return grid.NewScreen(8, 3) },
				"inline": func() regionCanvas { return grid.NewInline(8, 3) },
			} {
				t.Run(name, func(t *testing.T) {
					canvas := makeCanvas()
					terminal, err := ptytest.NewScreen(ptytest.Size{Cols: 8, Rows: 3})
					if err != nil {
						t.Fatal(err)
					}
					var output bytes.Buffer
					draw := func(regions []region) {
						t.Helper()
						view := canvas.Frame()
						view.Text(0, 0, "........", grid.Style{})
						for _, item := range regions {
							view.Paint(item.area, item.id, cellPaint(item.name))
						}
						if err := flushCanvas(canvas, &output); err != nil {
							t.Fatal(err)
						}
						if err := terminal.Apply(output.Bytes()); err != nil {
							t.Fatal(err)
						}
					}
					draw(test.before)
					output.Reset()
					draw(test.after)
					if got := terminal.Rows()[0]; got != test.want {
						t.Fatalf("composed row = %q, want %q", got, test.want)
					}
					output.Reset()
					draw(test.after)
					if output.Len() != 0 {
						t.Fatalf("unchanged composition produced %q", output.String())
					}
				})
			}
		})
	}
}

type cellPaint string

func (p cellPaint) Paint(w grid.PaintContext) error {
	size := w.Size()
	_, err := fmt.Fprintf(w, "%s\x1b[%dD", strings.Repeat(string(p), size.X), size.X)
	return err
}

func (cellPaint) Erase(grid.PaintContext) error { return nil }

func TestCellPaintersAndCellsShareOneComposition(t *testing.T) {
	for _, test := range []struct {
		name          string
		before, after string
		oldX, newX    int
		want          string
	}{
		{"remove painter", "base", "base", 1, -1, "base    "},
		{"move painter", "base", "base", 1, 2, "ba@e    "},
		{"change underlying cells", "base", "next", 1, 1, "n@xt    "},
		{"change another cell in the row", "base", "basX", 1, 1, "b@sX    "},
	} {
		t.Run(test.name, func(t *testing.T) {
			for name, makeCanvas := range map[string]func() regionCanvas{
				"screen": func() regionCanvas { return grid.NewScreen(8, 3) },
				"inline": func() regionCanvas { return grid.NewInline(8, 3) },
			} {
				t.Run(name, func(t *testing.T) {
					canvas := makeCanvas()
					terminal, err := ptytest.NewScreen(ptytest.Size{Cols: 8, Rows: 3})
					if err != nil {
						t.Fatal(err)
					}
					draw := func(cells string, x int) []byte {
						t.Helper()
						view := canvas.Frame()
						view.Text(0, 0, cells, grid.Style{})
						if x >= 0 {
							view.Paint(grid.Area(x, 0, 1, 1), 1, cellPaint("@"))
						}
						var output bytes.Buffer
						if err := flushCanvas(canvas, &output); err != nil {
							t.Fatal(err)
						}
						if err := terminal.Apply(output.Bytes()); err != nil {
							t.Fatal(err)
						}
						return output.Bytes()
					}
					draw(test.before, test.oldX)
					draw(test.after, test.newX)
					if got := terminal.Rows()[0]; got != test.want {
						t.Fatalf("composed row = %q, want %q", got, test.want)
					}
					if output := draw(test.after, test.newX); len(output) != 0 {
						t.Fatalf("unchanged composition wrote %q", output)
					}
				})
			}
		})
	}
}

type blankCanvasPainter struct{}

func (blankCanvasPainter) Paint(w grid.PaintContext) error {
	size := w.Size()
	var output strings.Builder
	for y := range size.Y {
		if y > 0 {
			output.WriteString("\x1b[B")
		}
		output.WriteString(strings.Repeat(" ", size.X))
		output.WriteByte('\r')
	}
	if size.Y > 1 {
		fmt.Fprintf(&output, "\x1b[%dA", size.Y-1)
	}
	_, err := io.WriteString(w, output.String())
	return err
}

func (blankCanvasPainter) Erase(w grid.PaintContext) error {
	_, err := io.WriteString(w, "\x1b[2J")
	return err
}

func TestPainterErasurePrecedesRestoringCells(t *testing.T) {
	for name, makeCanvas := range map[string]func() regionCanvas{
		"screen": func() regionCanvas { return grid.NewScreen(8, 3) },
		"inline": func() regionCanvas { return grid.NewInline(8, 3) },
	} {
		t.Run(name, func(t *testing.T) {
			canvas := makeCanvas()
			terminal, err := ptytest.NewScreen(ptytest.Size{Cols: 8, Rows: 3})
			if err != nil {
				t.Fatal(err)
			}
			for _, showPainter := range []bool{true, false} {
				view := canvas.Frame()
				view.Text(0, 0, "base", grid.Style{})
				if showPainter {
					view.Paint(grid.Area(0, 0, 8, 3), 1, blankCanvasPainter{})
				}
				var output bytes.Buffer
				if err := flushCanvas(canvas, &output); err != nil {
					t.Fatal(err)
				}
				if err := terminal.Apply(output.Bytes()); err != nil {
					t.Fatal(err)
				}
				want := "base    "
				if showPainter {
					want = "        "
				}
				if got := terminal.Rows()[0]; got != want {
					t.Fatalf("painter present %v: composed row = %q, want %q", showPainter, got, want)
				}
			}
		})
	}
}
