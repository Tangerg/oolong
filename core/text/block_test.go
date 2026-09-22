package text_test

import (
	"strconv"
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

func TestBlockConcurrentWidthsAndReturnedRowMutation(t *testing.T) {
	t.Parallel()
	block := text.NewBlock(text.BlockConfig{Lines: []text.Line{text.Of("hello world", grid.Style{})}, Wrap: true})
	for _, width := range []int{5, 20, 7, 5} {
		t.Run(strconv.Itoa(width), func(t *testing.T) {
			t.Parallel()
			for range 20 {
				rows := block.Rows(width)
				if len(rows) != block.HeightForWidth(width) {
					t.Fatal("inconsistent projection")
				}
				if len(rows) > 0 {
					rows[0].Text = "mutated"
				}
				if block.Rows(width)[0].Text == "mutated" {
					t.Fatal("cache exposed")
				}
				block.Draw(grid.NewSurface(width, 3).View())
			}
		})
	}
}
