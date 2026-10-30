package text

import (
	"image"
	"strings"

	"github.com/Tangerg/oolong/ui/core/grid"
)

// Affinity distinguishes the two visual caret locations at a soft wrap. It also
// chooses the side when an external byte offset falls inside a grapheme.
type Affinity uint8

// Affinity chooses the side of a soft visual break without changing logical bytes.
const (
	Forward Affinity = iota
	Backward
)

// Position addresses a byte boundary in one logical line. Positions belong to
// the document revision carried by their enclosing Layout or model request; a
// line index alone is not a persistent identity across edits.
type Position struct {
	Line     int
	Byte     int
	Affinity Affinity
}

// Range identifies two logical document positions; projection orders its endpoints.
type Range struct {
	Start Position
	End   Position
}

// WrapMode chooses wrapping while sharing one grapheme and terminal-column implementation.
type WrapMode uint8

const (
	// WrapWords preserves all logical characters, preferring breaks after spaces.
	// It is suitable for editable text. Paragraph-style whitespace consumption is
	// the distinct contract of Line.Wrap and is never silently applied to edits.
	WrapWords WrapMode = iota
	// WrapGraphemes breaks only between grapheme clusters without word preference.
	WrapGraphemes
	// WrapNone keeps each logical line in one visual row.
	WrapNone
)

// LayoutConfig supplies cleaned logical lines, target width and wrapping policy. Revision is caller-owned metadata.
type LayoutConfig struct {
	Lines    []string
	Width    int
	Wrap     WrapMode
	Revision uint64
}

// Layout is an immutable logical-text to cell projection. It owns terminal-safe
// lines, so byte positions refer to cleaned logical text, never unparsed format
// source. Rendering, hit testing, caret placement and selection use one wrap.
type Layout struct {
	lines    []string
	rows     []LayoutRow
	width    int
	revision uint64
}

// LayoutRow is one visual row with its logical byte interval and soft-break relationship.
type LayoutRow struct {
	Line   int
	From   int
	To     int
	Joined bool
	Width  int
	Text   string
}

// NewLayout prepares immutable visual rows from owned printable text. It panics for an unknown wrap mode.
func NewLayout(cfg LayoutConfig) *Layout {
	if cfg.Wrap > WrapNone {
		panic("text: invalid wrapping policy")
	}
	l := &Layout{width: max(0, cfg.Width), revision: cfg.Revision}
	l.lines = make([]string, len(cfg.Lines))
	for i, line := range cfg.Lines {
		l.lines[i] = strings.Clone(Printable(line))
		l.wrapLine(i, cfg.Wrap)
	}
	return l
}

// Revision returns the caller's document revision associated with this layout.
func (l *Layout) Revision() uint64 {
	if l == nil {
		return 0
	}
	return l.revision
}

// Width returns the requested nonnegative terminal-column width.
func (l *Layout) Width() int {
	if l == nil {
		return 0
	}
	return l.width
}

// Len returns the number of prepared visual rows.
func (l *Layout) Len() int {
	if l == nil {
		return 0
	}
	return len(l.rows)
}

// Row returns one visual row and whether its index is in bounds.
func (l *Layout) Row(index int) (LayoutRow, bool) {
	if l == nil || index < 0 || index >= len(l.rows) {
		return LayoutRow{}, false
	}
	row := l.rows[index]
	row.Text = strings.Clone(row.Text)
	return row, true
}

// Rows returns a detached copy of the prepared row descriptors.
func (l *Layout) Rows() []LayoutRow {
	if l == nil {
		return nil
	}
	rows := make([]LayoutRow, len(l.rows))
	for i := range rows {
		rows[i], _ = l.Row(i)
	}
	return rows
}

func (l *Layout) appendRow(line, from, to int) {
	content := l.lines[line][from:to]
	l.rows = append(l.rows, LayoutRow{Line: line, From: from, To: to, Joined: from > 0, Width: Width(content), Text: content})
}

func (l *Layout) wrapLine(index int, policy WrapMode) {
	line := l.lines[index]
	if l.width <= 0 || policy == WrapNone || line == "" {
		l.appendRow(index, 0, len(line))
		return
	}
	start, column, lastBreak := 0, 0, -1
	for at, cluster := range Clusters(line) {
		step := advanceCluster(cluster, column) - column
		for column+step > l.width && at > start {
			end := at
			if policy == WrapWords && lastBreak > start {
				end = lastBreak
			}
			l.appendRow(index, start, end)
			start = end
			column = ColumnOf(line[start:at], at-start)
			step = advanceCluster(cluster, column) - column
		}
		if cluster == " " {
			lastBreak = at + len(cluster)
		}
		column += step
	}
	l.appendRow(index, start, len(line))
}

// Normalize clamps a logical position to a valid line and grapheme boundary.
func (l *Layout) Normalize(position Position) Position {
	if l == nil || len(l.lines) == 0 {
		return Position{}
	}
	if position.Line < 0 {
		return Position{}
	}
	if position.Line >= len(l.lines) {
		return Position{Line: len(l.lines) - 1, Byte: len(l.lines[len(l.lines)-1]), Affinity: position.Affinity}
	}
	line := l.lines[position.Line]
	position.Byte = min(max(0, position.Byte), len(line))
	for at, cluster := range Clusters(line) {
		end := at + len(cluster)
		if position.Byte == at || position.Byte == end {
			return position
		}
		if position.Byte < end {
			position.Byte = at
			if position.Affinity != Backward {
				position.Byte = end
			}
			return position
		}
	}
	return position
}

// Caret maps a normalized logical position and affinity to visual cell coordinates.
func (l *Layout) Caret(position Position) image.Point {
	if l == nil || len(l.rows) == 0 {
		return image.Point{}
	}
	position = l.Normalize(position)
	for i, row := range l.rows {
		if row.Line != position.Line {
			continue
		}
		last := i == len(l.rows)-1 || l.rows[i+1].Line != row.Line
		if position.Byte < row.To || position.Byte == row.To && (position.Affinity == Backward || last) {
			return image.Pt(ColumnOf(row.Text, position.Byte-row.From), i)
		}
	}
	last := l.rows[len(l.rows)-1]
	return image.Pt(last.Width, len(l.rows)-1)
}

// Hit maps visual cell coordinates to a grapheme-aligned logical position.
func (l *Layout) Hit(point image.Point) Position {
	if l == nil || len(l.rows) == 0 {
		return Position{}
	}
	if point.Y >= len(l.rows) {
		last := l.rows[len(l.rows)-1]
		return Position{Line: last.Line, Byte: last.To, Affinity: Backward}
	}
	index := max(0, point.Y)
	row := l.rows[index]
	position := Position{Line: row.Line, Byte: row.From + OffsetAt(row.Text, max(0, point.X))}
	if position.Byte == row.To && index+1 < len(l.rows) && l.rows[index+1].Line == row.Line {
		position.Affinity = Backward
	}
	return l.Normalize(position)
}

func before(a, b Position) bool { return a.Line < b.Line || a.Line == b.Line && a.Byte < b.Byte }

func (l *Layout) normalizeRange(selected Range) Range {
	selected.Start, selected.End = l.Normalize(selected.Start), l.Normalize(selected.End)
	if before(selected.End, selected.Start) {
		selected.Start, selected.End = selected.End, selected.Start
	}
	return selected
}

// Rects returns cell intervals for the selected logical characters. A selected
// newline has one marker cell after its line when the width leaves room for it.
func (l *Layout) Rects(selected Range) []image.Rectangle {
	if l == nil {
		return nil
	}
	selected = l.normalizeRange(selected)
	if selected.Start.Line == selected.End.Line && selected.Start.Byte == selected.End.Byte {
		return nil
	}
	var rectangles []image.Rectangle
	for i, row := range l.rows {
		if row.Line < selected.Start.Line || row.Line > selected.End.Line {
			continue
		}
		from, to := row.From, row.To
		if row.Line == selected.Start.Line {
			from = max(from, selected.Start.Byte)
		}
		if row.Line == selected.End.Line {
			to = min(to, selected.End.Byte)
		}
		left, right := ColumnOf(row.Text, from-row.From), ColumnOf(row.Text, to-row.From)
		last := i == len(l.rows)-1 || l.rows[i+1].Line != row.Line
		if last && row.Line < selected.End.Line {
			right++
		}
		if l.width > 0 {
			left, right = min(left, l.width), min(right, l.width)
		}
		if right > left {
			rectangles = append(rectangles, image.Rect(left, i, right, i+1))
		}
	}
	return rectangles
}

// Text copies logical text, preserving hard newlines and omitting soft wraps.
func (l *Layout) Text(selected Range) string {
	if l == nil || len(l.lines) == 0 {
		return ""
	}
	selected = l.normalizeRange(selected)
	var out strings.Builder
	for line := selected.Start.Line; line <= selected.End.Line; line++ {
		if line != selected.Start.Line {
			out.WriteByte('\n')
		}
		from, to := 0, len(l.lines[line])
		if line == selected.Start.Line {
			from = selected.Start.Byte
		}
		if line == selected.End.Line {
			to = selected.End.Byte
		}
		out.WriteString(l.lines[line][from:to])
	}
	return out.String()
}

// Draw paints the same prepared rows used by hit testing, carets and selection.
func (l *Layout) Draw(view grid.View, style grid.Style) {
	if l == nil {
		return
	}
	visible := view.Visible()
	for y := max(0, visible.Min.Y); y < min(len(l.rows), visible.Max.Y); y++ {
		Of(l.rows[y].Text, style).Draw(view, 0, y)
	}
}
