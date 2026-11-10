package markdown

import (
	"strings"
	"testing"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/text"
)

func TestDividerPatternsOwnTheirTextBoundaries(t *testing.T) {
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
			drawRule(view.Sub(grid.Area(-test.clipped, 0, width+test.clipped, 1)), text.Span{Text: test.glyph})
			if got, want := surface.Rows()[0], "["+test.want+"]"; got != want {
				t.Fatalf("divider = %q, want %q", got, want)
			}
		})
	}
}

func TestDividerKeepsIndependentModifiersAtPartialTiles(t *testing.T) {
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
		drawRule(view, text.Span{Text: "\t🏻-", Style: style, Link: target})
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
