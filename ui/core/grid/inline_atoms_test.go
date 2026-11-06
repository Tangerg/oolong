package grid_test

import (
	"bytes"
	"slices"
	"testing"

	"github.com/Tangerg/oolong/ui/core/grid"
)

func TestInlineRowsKeepIndependentAtomsAtTheirSharedMargin(t *testing.T) {
	for _, test := range []struct {
		name, last, first string
	}{
		{"regional indicators", "🇨", "🇳"},
		{"skin tone", "👋", "🏽"},
		{"zero width joiner", "👨\u200d", "👩"},
		{"spacing extender", "あ", "ﾞ"},
		{"prepended character", "\u0600", "A"},
	} {
		t.Run(test.name, func(t *testing.T) {
			width := max(grid.ClusterWidth(test.last), grid.ClusterWidth(test.first))
			draw := func(view grid.View) {
				view.Text(width-grid.ClusterWidth(test.last), 0, test.last, grid.Style{})
				view.Text(0, 1, test.first, grid.Style{})
			}
			want := grid.NewSurface(width, 2)
			draw(want.View())
			canvas := grid.NewInline(width, 2)
			draw(canvas.Frame())
			var output bytes.Buffer
			if err := canvas.Flush(&output); err != nil {
				t.Fatal(err)
			}
			terminal := atomTerminal(t, width, 2)
			applyAtoms(t, terminal, output.Bytes())
			for row := range 2 {
				assertAtomRow(t, terminal, want.Row(row), 0, row)
			}
		})
	}
}

func TestOneColumnInlineFramesKeepTheirRelativeAnchor(t *testing.T) {
	canvas := grid.NewInline(1, 4)
	terminal := atomTerminal(t, 1, 4)
	applyAtoms(t, terminal, []byte("H\r\n"))
	for _, test := range []struct {
		rows []string
		want []string
	}{
		{[]string{"a", "b", "c"}, []string{"H", "a", "b", "c"}},
		{[]string{"a", "x", "c"}, []string{"H", "a", "x", "c"}},
		{[]string{"a"}, []string{"H", "a", " ", " "}},
	} {
		lines(test.rows...)(canvas.Frame())
		var output bytes.Buffer
		if err := canvas.Flush(&output); err != nil {
			t.Fatal(err)
		}
		applyAtoms(t, terminal, output.Bytes())
		if got := terminal.Rows(); !slices.Equal(got, test.want) {
			t.Fatalf("rows = %q, want %q", got, test.want)
		}
	}
	var output bytes.Buffer
	if err := canvas.Finish(&output); err != nil {
		t.Fatal(err)
	}
	applyAtoms(t, terminal, append(output.Bytes(), 'S'))
	if got, want := terminal.Rows(), []string{"H", "a", "S", " "}; !slices.Equal(got, want) {
		t.Fatalf("finished rows = %q, want %q", got, want)
	}
}
