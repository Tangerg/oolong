package kit_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/text"
	"github.com/Tangerg/oolong/ui/kit"
)

func TestParagraphProjectionPreservesAtomsAcrossPartialTabs(t *testing.T) {
	const modifier = "🏻"
	const target = "https://example.test/source"
	style := grid.Style{Attr: grid.Bold}
	for _, test := range []struct {
		width, row, column int
		tail               string
	}{
		{width: 11, row: 0, column: 8, tail: "X"},
		{width: 10, row: 1, column: 0, tail: "X"},
		{width: 9, row: 1, column: 0, tail: "X"},
		{width: 3, row: 3, column: 0, tail: "X"},
		{width: 5, row: 1, column: 3, tail: "XXXX"},
	} {
		paragraph := kit.NewParagraph("", grid.Style{})
		paragraph.SetText([]text.Line{{{Text: "\t"}, {Text: modifier + test.tail, Style: style, Link: target}}})
		rows := paragraph.Rows(test.width)
		actual := grid.NewSurface(test.width, len(rows))
		paragraph.Draw(actual.View())
		if cell := cellAt(actual, test.column, test.row); cell.Content() != modifier || cell.Width() != 2 || cell.Style != style || cell.Link != target {
			t.Fatalf("width %d: modifier cell = %+v", test.width, cell)
		}
		if cell := cellAt(actual, test.column+1, test.row); cell.Width() != 0 || cell.Link != target {
			t.Fatalf("width %d: modifier continuation = %+v", test.width, cell)
		}
		projected := grid.NewSurface(test.width, len(rows))
		var copied strings.Builder
		for y, row := range rows {
			if row.SourceLine != 1 || row.Joined != (y > 0) || row.Offset != 0 || row.Gap != "" {
				t.Fatalf("width %d: row %d metadata = %+v", test.width, y, row)
			}
			row.Content.Draw(projected.View(), row.Offset, y)
			copied.WriteString(row.Content.String())
			if !reflect.DeepEqual(projected.Row(y), actual.Row(y)) {
				t.Fatalf("width %d: row %d projection changed cells", test.width, y)
			}
		}
		if got := copied.String(); got != "        "+modifier+test.tail {
			t.Fatalf("width %d: copied %q", test.width, got)
		}
		for _, row := range rows {
			for i := range row.Content {
				row.Content[i] = text.Span{Text: "caller mutation"}
			}
		}
		after := grid.NewSurface(test.width, len(rows))
		paragraph.Draw(after.View())
		for y := range rows {
			if !reflect.DeepEqual(after.Row(y), actual.Row(y)) {
				t.Fatalf("width %d: public Rows mutated the cached wrap", test.width)
			}
		}
	}
}

func TestParagraphLinksUseTheRetainedAtomCoordinates(t *testing.T) {
	const target = "https://example.test/path"
	paragraph := kit.NewParagraph("", grid.Style{})
	paragraph.SetText([]text.Line{{
		{Text: "        "},
		{Text: "🏻", BreakBefore: true},
		{Text: " " + target},
	}})
	paragraph.SetLinks(kit.LinkConfig{Enabled: true})
	surface := grid.NewSurface(50, 1)
	paragraph.Draw(surface.View())
	const firstColumn = 11
	if cell := cellAt(surface, firstColumn, 0); cell.Content() != "h" || cell.Link != target {
		t.Fatalf("first link cell = %+v", cell)
	}
	if destination, ok := paragraph.LinkAt(firstColumn, 0, 50); !ok || destination.Target != target {
		t.Fatalf("link hit = %+v, %t", destination, ok)
	}
	if _, ok := paragraph.LinkAt(firstColumn-1, 0, 50); ok || cellAt(surface, firstColumn-1, 0).Link != "" {
		t.Fatal("the link claimed the space preceding it")
	}
}
