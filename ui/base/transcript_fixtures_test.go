package base_test

import (
	"fmt"
	"strings"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/text"
)

type block struct {
	name           string
	lines, measure int
}

func (b *block) HeightForWidth(width int) int {
	b.measure++
	if width <= 0 {
		return 0
	}
	if width < 10 {
		return b.lines * 2
	}
	return b.lines
}

func (b *block) Draw(view grid.View) {
	for y := range min(b.lines, view.Visible().Max.Y) {
		view.Text(0, y, fmt.Sprintf("%s:%d", b.name, y), grid.Style{})
	}
}

func (b *block) Rows(width int) []text.Row {
	n := b.HeightForWidth(width)
	if n > 10000 {
		return nil
	}
	out := make([]text.Row, n)
	for y := range n {
		out[y] = text.Row{Text: fmt.Sprintf("%s:%d", b.name, y)}
	}
	return out
}

type plain struct{ lines int }

func (p *plain) HeightForWidth(int) int { return p.lines }
func (p *plain) Draw(view grid.View)    { view.Text(0, 0, "plain", grid.Style{}) }
func rowText(view grid.View, y int) string {
	width, _ := view.Size()
	var out strings.Builder
	for x := range width {
		c, _ := view.CellAt(x, y)
		if c.Width() > 0 {
			if c.Content() == "" {
				out.WriteByte(' ')
			} else {
				out.WriteString(c.Content())
			}
		}
	}
	return strings.TrimRight(out.String(), " ")
}

func rowTexts(rows []text.Row) []string {
	out := make([]string, len(rows))
	for i, row := range rows {
		out[i] = row.Text
	}
	return out
}
