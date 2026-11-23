package text_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/text"
)

func TestTileRepeatsItsProjectedPatternWithinTheVisibleClip(t *testing.T) {
	for _, test := range []struct {
		name, source, shown string
		width               int
	}{
		{name: "tab", source: "\t-", shown: "        -", width: 9},
		{name: "separated variation selector", source: "❤\n\ufe0fX", shown: "❤X", width: 2},
		{name: "wide cluster", source: "界-", shown: "界-", width: 3},
		{name: "leading combining mark", source: "\u0301-", shown: "-", width: 1},
		{name: "invisible", source: "\n\u0301"},
	} {
		t.Run(test.name, func(t *testing.T) {
			for _, offset := range []int{0, 1, 1_000_003} {
				actual := grid.NewSurface(20, 1)
				expected := grid.NewSurface(20, 1)
				area := grid.Area(-offset, 0, offset+20, 1)
				text.Of(test.source, grid.Style{}).Tile(actual.View().Sub(area), 0)
				if test.width > 0 {
					view := expected.View().Sub(area)
					for x := offset - offset%test.width; x < offset+20; x += test.width {
						view.Text(x, 0, test.shown, grid.Style{})
					}
				}
				if got, want := actual.Rows(), expected.Rows(); !reflect.DeepEqual(got, want) {
					t.Fatalf("offset %d: tile = %q, want %q", offset, got, want)
				}
			}
		})
	}
}

func TestTileKeepsTabOriginsAndIndependentMarks(t *testing.T) {
	for _, test := range []struct {
		name, glyph, want string
		clipped           int
	}{
		{name: "standalone combining marks", glyph: "\u0301-", want: "-----"},
		{name: "clipped combining marks", glyph: "\u0301-", clipped: 1, want: "-----"},
		{name: "tab origin", glyph: "\t-", want: "        -        -  "},
		{name: "clipped tab origin", glyph: "\t-", clipped: 1, want: "       -        -   "},
		{name: "clipped tab and independent mark", glyph: "\t\u0301-", clipped: 6, want: "  -        -        "},
	} {
		t.Run(test.name, func(t *testing.T) {
			width := len(test.want)
			surface := grid.NewSurface(width+2, 1)
			surface.View().Text(0, 0, "["+strings.Repeat(" ", width)+"]", grid.Style{})
			view := surface.View().Sub(grid.Area(1, 0, width, 1))
			text.Line{{Text: test.glyph}}.Tile(view.Sub(grid.Area(-test.clipped, 0, width+test.clipped, 1)), 0)
			if got, want := surface.Rows()[0], "["+test.want+"]"; got != want {
				t.Fatalf("tile = %q, want %q", got, want)
			}
		})
	}
}

// A repetition cut by the right edge keeps what fits whole and drops a wide cluster
// that would straddle it, keeping the cluster's style and link where it is drawn.
func TestTileCutsItsLastRepetitionAtTheEdge(t *testing.T) {
	style := grid.Style{Attr: grid.Bold}
	const target = "https://example.test/divider"
	for _, test := range []struct {
		width, offset int
		modifiers     []int
		dashes        []int
	}{
		{width: 10, modifiers: []int{8}},
		{width: 9},
		{width: 15, offset: 6, modifiers: []int{2, 13}, dashes: []int{4}},
	} {
		surface := grid.NewSurface(test.width, 1)
		view := surface.View().Sub(grid.Area(-test.offset, 0, test.width+test.offset, 1))
		text.Line{{Text: "\t🏻-", Style: style, Link: target}}.Tile(view, 0)
		for _, x := range test.modifiers {
			cell, _ := surface.CellAt(x, 0)
			if cell.Content() != "🏻" || cell.Width() != 2 || cell.Style != style || cell.Link != target {
				t.Fatalf("width %d offset %d: modifier at %d = %+v", test.width, test.offset, x, cell)
			}
			continuation, _ := surface.CellAt(x+1, 0)
			if continuation.Width() != 0 || continuation.Link != target {
				t.Fatalf("modifier continuation = %+v", continuation)
			}
		}
		for _, x := range test.dashes {
			cell, _ := surface.CellAt(x, 0)
			if cell.Content() != "-" || cell.Link != target {
				t.Fatalf("divider at %d = %+v", x, cell)
			}
		}
		if len(test.modifiers) == 0 && surface.Rows()[0] != "" {
			t.Fatalf("a partial modifier survived: %q", surface.Rows())
		}
	}
}
