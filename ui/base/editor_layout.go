package base

import "github.com/Tangerg/oolong/ui/core/text"

type editorRow struct {
	line       int
	start, end int
	joined     bool
}

// The editor retains one immutable projection; wrapping and byte/cell conversion
// belong to text.Layout and cannot diverge between editing and selection.
type editorLayout struct {
	rows       []editorRow
	projection *text.Layout
	width      int
	stale      bool
}

func (l *editorLayout) rowsFor(lines []string, width int) []editorRow {
	if !l.stale && l.width == width && l.rows != nil {
		return l.rows
	}
	l.projection = text.NewLayout(text.LayoutConfig{Lines: lines, Width: width})
	l.rows = make([]editorRow, l.projection.Len())
	for i, row := range l.projection.Rows() {
		l.rows[i] = editorRow{line: row.Line, start: row.From, end: row.To, joined: row.Joined}
	}
	l.width, l.stale = width, false
	return l.rows
}

func (l *editorLayout) lastOfLine(i int) bool {
	return i == len(l.rows)-1 || l.rows[i+1].line != l.rows[i].line
}
