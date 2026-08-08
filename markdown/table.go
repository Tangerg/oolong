package markdown

import (
	"strings"

	east "github.com/yuin/goldmark/extension/ast"

	"github.com/Tangerg/oolong/core/text"
)

// table renders a table as columns padded to their widest cell.
//
// It is laid out here rather than left to whatever draws the document, because a
// table is the one block whose shape depends on all of its own contents at once: how
// wide a column is cannot be known from one row. What comes out is ordinary lines,
// so a table wraps like anything else when it is drawn somewhere too narrow for it —
// which is ugly and is still better than cutting the last column off.
func (r *renderer) table(n *east.Table, in frame) {
	rows, header := r.cells(n)
	if len(rows) == 0 {
		return
	}
	widths := make([]int, 0, 8)
	for _, row := range rows {
		for i, cell := range row {
			if i >= len(widths) {
				widths = append(widths, 0)
			}
			widths[i] = max(widths[i], cell.Width())
		}
	}

	lines := make([]text.Line, 0, len(rows)+1)
	for i, row := range rows {
		lines = append(lines, r.tableRow(row, widths, n.Alignments))
		if i == 0 && header {
			lines = append(lines, r.tableRule(widths))
		}
	}
	r.push(Block{Indent: in.indent, Rail: in.rail.line(), Gap: !in.tight, Lines: lines})
}

// cells reads the table into lines, and says whether the first row is the heading.
func (r *renderer) cells(n *east.Table) (rows [][]text.Line, header bool) {
	for child := n.FirstChild(); child != nil; child = child.NextSibling() {
		style := r.look.Text
		if _, is := child.(*east.TableHeader); is {
			style = r.look.Strong
			header = header || len(rows) == 0
		}
		row := make([]text.Line, 0, 8)
		for cell := child.FirstChild(); cell != nil; cell = cell.NextSibling() {
			// A cell is one line: a line break inside one is not something the syntax can
			// express, so the first line is the whole of it.
			row = append(row, r.inline(cell, style)[0])
		}
		rows = append(rows, row)
	}
	return rows, header
}

// tableRow pads each cell to its column's width and joins them.
func (r *renderer) tableRow(row []text.Line, widths []int, aligns []east.Alignment) text.Line {
	out := make(text.Line, 0, len(row)*2)
	for i, cell := range row {
		if i > 0 {
			out = append(out, text.Span{Text: r.column(), Style: r.look.Rail})
		}
		pad := widths[i] - cell.Width()
		left, right := 0, pad
		switch align(aligns, i) {
		case east.AlignRight:
			left, right = pad, 0
		case east.AlignCenter:
			left, right = pad/2, pad-pad/2
		case east.AlignLeft, east.AlignNone:
			// Which is where the padding already is.
		}
		out = append(out, blank(left))
		out = append(out, cell...)
		out = append(out, blank(right))
	}
	return out
}

// tableRule is the line under the heading.
func (r *renderer) tableRule(widths []int) text.Line {
	divider := r.look.Glyphs.Divider
	if divider == "" {
		divider = " "
	}
	out := make(text.Line, 0, len(widths)*2)
	for i, width := range widths {
		if i > 0 {
			out = append(out, text.Span{Text: r.column(), Style: r.look.Rail})
		}
		out = append(out, text.Span{Text: strings.Repeat(divider, width), Style: r.look.Rule})
	}
	return out
}

// column is what goes between two cells: the look's bar where it has one, and room
// where it does not — because two columns run together is worse than a table with no
// lines in it.
func (r *renderer) column() string {
	if r.look.Glyphs.Bar == "" {
		return "  "
	}
	return " " + r.look.Glyphs.Bar + " "
}

// align is a column's alignment, or none for a column the table said nothing about.
func align(aligns []east.Alignment, column int) east.Alignment {
	if column < 0 || column >= len(aligns) {
		return east.AlignNone
	}
	return aligns[column]
}

// blank is n columns of nothing.
func blank(n int) text.Span {
	if n <= 0 {
		return text.Span{}
	}
	return text.Span{Text: strings.Repeat(" ", n)}
}
