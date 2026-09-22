package text_test

import (
	"testing"

	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/text"
)

func TestBlockOwnsSourceAndSharesLayoutWithProjection(t *testing.T) {
	lines := []text.Line{{{Text: "hello world"}}}
	block := text.NewBlock(text.BlockConfig{Lines: lines, Wrap: true})
	lines[0][0].Text = "changed"
	for _, width := range []int{0, 1, 5, 20} {
		rows := block.Rows(width)
		if block.HeightForWidth(width) != len(rows) {
			t.Fatal("height and rows disagree")
		}
		if width == 0 {
			continue
		}
		surface := grid.NewSurface(width, len(rows))
		block.Draw(surface.View())
		for y, row := range rows {
			for x, r := range row.Text {
				cell, ok := surface.CellAt(x, y)
				if !ok || cell.Content() != string(r) {
					t.Fatalf("width=%d row=%d text=%q", width, y, row.Text)
				}
			}
		}
	}
	if got := block.Rows(20)[0].Text; got != "hello world" {
		t.Fatal(got)
	}
	clipped := text.NewBlock(text.BlockConfig{Lines: []text.Line{{{Text: "abcdef"}}}})
	if rows := clipped.Rows(3); len(rows) != 1 || rows[0].Text != "abc" {
		t.Fatal(rows)
	}
}
