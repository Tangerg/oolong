package text_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/text"
)

func TestTabExpansionCannotGiveInvisibleAtomsASpaceBase(t *testing.T) {
	for _, modifier := range []string{"\u0301", "\ufe0f", "\u200d", "\u200b\u0301"} {
		for _, prefix := range []string{"", "a", "界", "a\tb"} {
			source := prefix + "\t" + modifier + "XY"
			t.Run(source, func(t *testing.T) {
				line := text.Of(source, grid.Style{})
				for budget := 1; budget <= line.Width(); budget++ {
					want := grid.Render(budget, 1, func(view grid.View) { line.Draw(view, 0, 0) })
					for name, projected := range map[string]text.Line{
						"truncate": line.Truncate(budget, ""),
						"suffix":   line.WithSuffix(budget, ""),
						"wrap":     line.Wrap(0)[0].Line,
					} {
						got := grid.Render(budget, 1, func(view grid.View) { projected.Draw(view, 0, 0) })
						if !slices.Equal(got, want) {
							t.Fatalf("%s at width %d shows %q, want %q", name, budget, got, want)
						}
					}
				}
			})
		}
	}
}

func TestTabExpansionPreservesFollowingAppearanceAndSourceIntervals(t *testing.T) {
	const source = "\t\u0301X Y"
	const target = "https://example.test/following"
	style := grid.Style{Attr: grid.Bold}
	line := text.Line{{Text: "\t"}, {Text: "\u0301X Y", Style: style, Link: target}}
	rows := line.Wrap(text.TabStop + 1)
	if len(rows) != 2 || rows[0].Line.String() != strings.Repeat(" ", text.TabStop)+"\u0301X" || rows[1].Line.String() != "Y" {
		t.Fatalf("wrapped rows = %+v", rows)
	}
	if rows[0].From != 0 || rows[0].To != len("\t\u0301X") || rows[0].Joined || rows[0].Gap != "" ||
		rows[1].From != len("\t\u0301X ") || rows[1].To != len(source) || !rows[1].Joined || rows[1].Gap != " " {
		t.Fatalf("wrapped source intervals = %+v", rows)
	}
	for name, projected := range map[string]text.Line{
		"truncate": line.Truncate(text.TabStop+1, ""),
		"suffix":   line.WithSuffix(text.TabStop+1, ""),
		"wrap":     rows[0].Line,
	} {
		surface := grid.NewSurface(text.TabStop+1, 1)
		projected.Draw(surface.View(), 0, 0)
		if cell := cellAt(surface, text.TabStop); cell.Content() != "X" || cell.Style != style || cell.Link != target {
			t.Fatalf("%s: following cell = %+v", name, cell)
		}
		if cell := cellAt(surface, text.TabStop-1); cell.Content() != " " || cell.Link != "" {
			t.Fatalf("%s: expanded tab cell = %+v", name, cell)
		}
	}
}
