package markdown_test

import (
	"reflect"
	"testing"

	"github.com/Tangerg/oolong/extensions/markdown"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/text"
)

func TestEmbeddedRowsKeepAtomGeometryAndSnapshotOwnership(t *testing.T) {
	const target = "https://example.test/embedded"
	style := grid.Style{Attr: grid.Bold}
	body := text.NewBlock(text.BlockConfig{
		Lines: []text.Line{{{Text: "\t"}, {Text: "🏻XXXX", Style: style, Link: target}}},
		Wrap:  true,
	})
	var look markdown.Look
	look.SetRenderer(markdown.FencedCode, func(string, string) (grid.Drawable, error) { return body, nil })
	blocks, err := markdown.Render("```atoms\nsource\n```", look)
	if err != nil {
		t.Fatal(err)
	}
	var doc markdown.Doc
	doc.SetBlocks(blocks)
	const width = 5
	rows := doc.Rows(width)
	surface := grid.NewSurface(width, len(rows))
	doc.Draw(surface.View())
	modifier := cellAt(surface, 3, 1)
	if modifier.Content() != "🏻" || modifier.Width() != 2 || modifier.Style != style || modifier.Link != target {
		t.Fatalf("partial-tab modifier = %+v", modifier)
	}
	projected := grid.NewSurface(width, len(rows))
	for y, row := range rows {
		if row.SourceLine != 1 || row.Joined != (y > 0) || row.Offset != 0 || row.Gap != "" {
			t.Fatalf("row %d metadata = %+v", y, row)
		}
		row.Content.Draw(projected.View(), row.Offset, y)
		if !reflect.DeepEqual(projected.Row(y), surface.Row(y)) {
			t.Fatalf("row %d changed through Markdown projection", y)
		}
		for i := range row.Content {
			row.Content[i] = text.Span{Text: "caller mutation"}
		}
	}
	after := grid.NewSurface(width, len(rows))
	doc.Draw(after.View())
	for y, row := range doc.Rows(width) {
		if !reflect.DeepEqual(after.Row(y), surface.Row(y)) {
			t.Fatalf("public rows mutated drawing row %d", y)
		}
		projected.Reset()
		row.Content.Draw(projected.View(), row.Offset, y)
		if !reflect.DeepEqual(projected.Row(y), surface.Row(y)) {
			t.Fatalf("public rows mutated the cached row %d", y)
		}
	}
}
