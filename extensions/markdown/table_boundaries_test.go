package markdown_test

import (
	"strings"
	"testing"

	"github.com/Tangerg/oolong/extensions/markdown"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/text"
)

func TestTableCellsKeepColumnsAcrossGraphemeBoundaries(t *testing.T) {
	for _, value := range []string{"\u0600\u0600", "\u0600", "🏽", "\u200d💻", "ाा", "👩🏽‍💻", "🇨🇳", "e\u0301"} {
		for _, bar := range []string{"│", "\u0600", "\u0600\u0600", "\u0301", "ाा"} {
			for _, alignment := range []string{"---", ":---:", "---:"} {
				t.Run(value+"/"+bar+"/"+alignment, func(t *testing.T) {
					const width = 24
					rail := grid.Style{Attr: grid.Underline}
					look := markdown.Look{Rail: rail, Glyphs: markdown.Glyphs{Bar: bar, Divider: "─"}}
					source := "| head | z |\n| " + alignment + " | --- |\n| " + value + " | z |\n"
					blocks, err := markdown.Render(source, look)
					if err != nil {
						t.Fatal(err)
					}
					if len(blocks) != 1 {
						t.Fatalf("got %d blocks, want one table", len(blocks))
					}
					rows := blocks[0].Rows(width)
					if len(rows) != 3 {
						t.Fatalf("got %d rows, want a header, rule and record", len(rows))
					}
					if !strings.Contains(rows[2].Content.String(), value) {
						t.Fatalf("cell content was lost: %q", rows[2].Content.String())
					}
					drawn := grid.NewSurface(width, len(rows))
					blocks[0].Draw(drawn.View())
					column := func(y int, content string, style grid.Style) int {
						for x, cell := range drawn.Row(y) {
							if cell.Style == style && strings.Contains(cell.Content(), content) {
								return x
							}
						}
						t.Fatalf("row %d has no %q with style %+v: %q", y, content, style, rows[y].Content.String())
						return 0
					}
					if got, want := column(2, "z", grid.Style{}), column(0, "z", grid.Style{}); got != want {
						t.Errorf("second cell starts in column %d, want %d", got, want)
					}
					if got, want := column(2, bar, rail), column(0, bar, rail); got != want {
						t.Errorf("separator starts in column %d, want %d", got, want)
					}
					if got, want := rows[2].Content.Width(), rows[0].Content.Width(); got != want {
						t.Errorf("record width = %d, want header width %d", got, want)
					}
					projected := grid.NewSurface(width, len(rows))
					for y, row := range rows {
						row.Content.Draw(projected.View(), row.Offset, y)
						for x, actual := range drawn.Row(y) {
							want := cellAt(projected, x, y)
							if actual.Content() != want.Content() || actual.Width() != want.Width() {
								t.Fatalf("Rows and Draw differ at (%d,%d): %+v versus %+v", x, y, actual, want)
							}
						}
					}
				})
			}
		}
	}
}

func TestTableColumnsKeepIndivisibleGraphemesWhole(t *testing.T) {
	cluster := "ｶ" + strings.Repeat("ﾞ", 11)
	source := "| symbol | other |\n| --- | --- |\n| " + cluster + " | abcdefghijabcdefghijabcdefghij |\n"
	blocks, err := markdown.Render(source, look())
	if err != nil {
		t.Fatal(err)
	}
	for _, width := range []int{20, 18} {
		rows := blocks[0].Rows(width)
		found := false
		for _, row := range rows {
			if got := row.Content.Width(); got > width {
				t.Fatalf("table at width %d produced a %d-column row: %q", width, got, row.Content.String())
			}
			found = found || strings.Contains(row.Content.String(), cluster)
		}
		if !found {
			t.Fatalf("table at width %d lost its complete grapheme: %+v", width, rows)
		}
		if width == 20 {
			for _, row := range rows {
				before, _, found := strings.Cut(row.Content.String(), "│")
				if !found || text.Width(before) != text.Width(cluster)+1 {
					t.Fatalf("wide atom moved the grid separator: %q", row.Content.String())
				}
			}
		} else if strings.Contains(rows[0].Content.String(), "│") {
			t.Fatalf("grid remained after its indivisible columns stopped fitting: %+v", rows)
		}
	}
}
