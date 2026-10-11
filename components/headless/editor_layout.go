package headless

import (
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/text"
)

type editorRow struct {
	line       int
	start, end int
	joined     bool
}

// Cursor routing and selection painting need byte offsets from the same wrap used
// for drawing; independent wraps disagree on wide graphemes and soft breaks.
type editorLayout struct {
	rows  []editorRow
	width int
	stale bool
}

func (l *editorLayout) rowsFor(lines []string, width int) []editorRow {
	if !l.stale && l.width == width && l.rows != nil {
		return l.rows
	}
	l.rows = l.rows[:0]
	for i, line := range lines {
		l.wrapLine(i, line, width)
	}
	l.width, l.stale = width, false
	return l.rows
}

func (l *editorLayout) wrapLine(index int, line string, width int) {
	if width <= 0 || line == "" {
		l.rows = append(l.rows, editorRow{line: index, start: 0, end: len(line)})
		return
	}

	start := 0
	column := 0
	lastBreak := -1
	for at, cluster := range text.Clusters(line) {
		step := grid.ClusterWidth(cluster)
		if cluster == "\t" {
			step = text.TabStop - column%text.TabStop
		}
		for column+step > width && at > start {
			end := at
			if lastBreak > start {
				end = lastBreak
			}
			l.rows = append(l.rows, editorRow{
				line: index, start: start, end: end, joined: start > 0,
			})
			start = end
			column = text.ColumnOf(line[start:at], at-start)
			if cluster == "\t" {
				step = text.TabStop - column%text.TabStop
			}
		}
		if cluster == " " {
			lastBreak = at + len(cluster)
		}
		column += step
	}
	l.rows = append(l.rows, editorRow{line: index, start: start, end: len(line), joined: start > 0})
}

func (l *editorLayout) lastOfLine(i int) bool {
	return i == len(l.rows)-1 || l.rows[i+1].line != l.rows[i].line
}
