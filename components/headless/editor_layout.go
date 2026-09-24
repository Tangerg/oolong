package headless

import (
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/text"
)

// editorRow is one visual row: which logical line it came from, and the slice of
// that line it shows.
type editorRow struct {
	line       int
	start, end int
	// joined marks a row that continues the line above rather than starting one.
	joined bool
}

// editorLayout is the wrap of an editor's text at one width.
//
// The editor wraps its own text rather than asking the text package to, because it
// needs the byte offset each row starts at: the cursor lives at an offset, and
// moving it down the screen means finding the offset that sits under it. Measuring
// with one wrap and drawing with another is how a cursor ends up a column away from
// the character it is on.
type editorLayout struct {
	rows  []editorRow
	width int
	stale bool
}

// rowsFor lays the lines out at a width, reusing the last layout when nothing that
// matters has changed.
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

// wrapLine appends the rows one logical line occupies.
//
// It breaks at spaces when it can and between clusters when it cannot, which is what
// the text package does, so a field and the prose beside it break in the same
// places. A line with nothing in it still gets a row: a blank line in a composer is
// a blank line on screen.
func (l *editorLayout) wrapLine(index int, line string, width int) {
	if width <= 0 || line == "" {
		l.rows = append(l.rows, editorRow{line: index, start: 0, end: len(line)})
		return
	}

	start := 0
	column := 0
	// lastBreak is where the row could end instead, and lastBreakColumn what it
	// would be worth: the offset after the most recent space.
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
