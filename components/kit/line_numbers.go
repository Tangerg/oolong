package kit

import (
	"strconv"
	"strings"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/text"
)

// LineNumbers draws the logical source line beside visual rows.
//
// Continuation rows are deliberately blank: a number names a logical line, not
// every row the current width happened to wrap it into. It implements
// [headless.RowGutter], so the same value dresses an editor and a passive [Code]
// block without either component learning the other's layout.
type LineNumbers struct {
	Style grid.Style
	// First is the number of source line one. Zero uses one, which is how a whole
	// document is numbered; a snippet beginning later sets the actual first line.
	First int
	// Separator is drawn after the number column. Empty draws none.
	Separator string
	// Gap is the clear space after Separator. Zero uses one column.
	Gap int
}

var _ headless.RowGutter = LineNumbers{}

// Width is the stable gutter width for this many logical lines.
func (n LineNumbers) Width(lines int) int {
	last := n.first() + max(lines-1, 0)
	return decimalWidth(last) + text.Width(n.Separator) + n.gap()
}

// Draw paints one number for each logical line beginning in rows.
func (n LineNumbers) Draw(view grid.View, rows []text.Row) {
	width, height := view.Size()
	if width <= 0 || height <= 0 {
		return
	}
	digits := max(width-text.Width(n.Separator)-n.gap(), 0)
	for y, row := range rows {
		if y >= height {
			return
		}
		if row.Line <= 0 || row.Joined {
			continue
		}
		value := n.first() + row.Line - 1
		label := right(decimal(value), digits) + n.Separator
		view.Text(0, y, text.Truncate(label, width, ""), n.Style)
	}
}

func (n LineNumbers) first() int { return max(n.First, 1) }

func (n LineNumbers) gap() int { return max(n.Gap, 1) }

func decimal(n int) string {
	if n <= 0 {
		return ""
	}
	return strconv.Itoa(n)
}

func decimalWidth(n int) int { return len(decimal(max(n, 1))) }

func right(s string, width int) string {
	return strings.Repeat(" ", max(width-text.Width(s), 0)) + s
}
