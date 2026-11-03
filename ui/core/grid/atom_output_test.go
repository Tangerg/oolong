package grid_test

import (
	"bytes"
	"testing"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/uitest/ptytest"
)

func TestIndependentCellAtomsPreserveTheirWireCoordinates(t *testing.T) {
	for _, test := range []struct {
		name, left, right string
	}{
		{name: "skin tone", left: "👋", right: "🏽"},
		{name: "zero width joiner", left: "👨", right: "\u200d👩"},
		{name: "regional indicators", left: "🇨", right: "🇳"},
		{name: "spacing extender", left: "あ", right: "ﾞ"},
		{name: "prepended character", left: "\u0600", right: "A"},
	} {
		for _, differingStyle := range []bool{false, true} {
			name := test.name + "/same style"
			if differingStyle {
				name = test.name + "/different style"
			}
			t.Run(name, func(t *testing.T) {
				const width = 10
				surface := grid.NewSurface(width, 1)
				draw := func(view grid.View) {
					column := view.Text(0, 0, test.left, grid.Style{})
					style := grid.Style{}
					if differingStyle {
						style.Attr = grid.Bold
					}
					column += view.Text(column, 0, test.right, style)
					view.Text(column, 0, "X", grid.Style{})
				}
				draw(surface.View())
				want := surface.Row(0)
				for _, initial := range []string{"", "previous"} {
					canvas := grid.NewScreen(width, 1)
					terminal := atomTerminal(t, width, 1)
					if initial != "" {
						canvas.Frame().Text(0, 0, initial, grid.Style{})
						var output bytes.Buffer
						if err := canvas.Flush(&output); err != nil {
							t.Fatal(err)
						}
						applyAtoms(t, terminal, output.Bytes())
					}
					draw(canvas.Frame())
					var output bytes.Buffer
					if err := canvas.Flush(&output); err != nil {
						t.Fatal(err)
					}
					applyAtoms(t, terminal, output.Bytes())
					assertAtomRow(t, terminal, want, 0, 0)
					draw(canvas.Frame())
					output.Reset()
					if err := canvas.Flush(&output); err != nil || output.Len() != 0 {
						t.Fatalf("unchanged frame wrote %q, error = %v", output.Bytes(), err)
					}
				}
				terminal := atomTerminal(t, width+2, 1)
				const positionAfterPrefix = "\x1b[1;3H"
				applyAtoms(t, terminal, []byte("ab"+positionAfterPrefix+grid.EncodeRow(want, grid.TrueColor)))
				assertAtomRow(t, terminal, want, 2, 0)
				inline := grid.NewInline(width, 1)
				draw(inline.Frame())
				var output bytes.Buffer
				if err := inline.Flush(&output); err != nil {
					t.Fatal(err)
				}
				terminal = atomTerminal(t, width, 1)
				applyAtoms(t, terminal, output.Bytes())
				assertAtomRow(t, terminal, want, 0, 0)
			})
		}
	}
}

func TestFrozenAtomBoundariesSurviveInlineReflow(t *testing.T) {
	frozen, err := grid.Capture(grid.CaptureConfig{
		Width: 5,
		Content: content(1, func(view grid.View) {
			view.Text(0, 0, "👋", grid.Style{})
			view.Text(2, 0, "🏽", grid.Style{})
			view.Text(4, 0, "X", grid.Style{})
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, width := range []int{3, 5} {
		rows, err := frozen.Rows(width)
		if err != nil {
			t.Fatal(err)
		}
		inline := grid.NewInline(width, 1)
		inline.Frame().Text(0, 0, "L", grid.Style{})
		var output bytes.Buffer
		if err := inline.Flush(&output, frozen); err != nil {
			t.Fatal(err)
		}
		terminal := atomTerminal(t, width, len(rows)+1)
		applyAtoms(t, terminal, output.Bytes())
		for y, row := range rows {
			assertAtomRow(t, terminal, row, 0, y)
		}
		if got := terminal.At(0, len(rows)); got != "L" {
			t.Fatalf("width %d moved the live anchor: marker = %q, rows = %q", width, got, terminal.Rows())
		}
	}
}

func TestZeroWidthExtensionPreservesTheCompleteAtomsAppearance(t *testing.T) {
	style := grid.Style{FG: grid.RGBColor(90, 120, 160), Attr: grid.Bold}
	const target = "https://example.test/atom"
	for _, base := range []string{"e", "👨", "あﾞ"} {
		t.Run(base, func(t *testing.T) {
			frozen, err := grid.Capture(grid.CaptureConfig{
				Width: 6,
				Content: content(1, func(view grid.View) {
					width := view.Text(0, 0, base, style)
					view.Link(0, 0, width, target)
					width += view.Text(width, 0, "\u200d", grid.Style{Attr: grid.Underline})
					view.Text(width, 0, "X", grid.Style{})
				}),
			})
			if err != nil {
				t.Fatal(err)
			}
			rows, err := frozen.Rows(6)
			if err != nil {
				t.Fatal(err)
			}
			row := rows[0]
			width := grid.ClusterWidth(base + "\u200d")
			if row[0].Content() != base+"\u200d" || row[0].Width() != width || row[width].Content() != "X" {
				t.Fatalf("extended atom geometry = %+v", row)
			}
			for column := range width {
				if row[column].Style != style || row[column].Link != target {
					t.Fatalf("column %d lost the atom's appearance: %+v", column, row[column])
				}
			}
			terminal := atomTerminal(t, 6, 1)
			applyAtoms(t, terminal, []byte(grid.EncodeRow(row, grid.TrueColor)))
			assertAtomRow(t, terminal, row, 0, 0)
		})
	}
}

func atomTerminal(t *testing.T, width, height int) *ptytest.Screen {
	t.Helper()
	terminal, err := ptytest.NewScreen(ptytest.Size{Cols: width, Rows: height})
	if err != nil {
		t.Fatal(err)
	}
	return terminal
}

func applyAtoms(t *testing.T, terminal *ptytest.Screen, output []byte) {
	t.Helper()
	if err := terminal.Apply(output); err != nil {
		t.Fatal(err)
	}
	if err := terminal.Flush(); err != nil {
		t.Fatal(err)
	}
}

func assertAtomRow(t *testing.T, terminal *ptytest.Screen, want []grid.Cell, column, row int) {
	t.Helper()
	for x, cell := range want {
		got := terminal.At(column+x, row)
		if cell.Blank() {
			if got != "" && got != " " {
				t.Errorf("blank at (%d,%d) = %q", column+x, row, got)
			}
			continue
		}
		if got != cell.Content() {
			t.Errorf("atom at (%d,%d) = %q, want %q; rows = %q", column+x, row, got, cell.Content(), terminal.Rows())
		}
	}
}
