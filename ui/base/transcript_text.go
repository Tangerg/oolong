package base

import (
	"crypto/sha256"
	"image"
	"strings"

	"github.com/Tangerg/oolong/ui/core/text"
)

// transcriptTextMap projects a renderer's meaningful rows. Renderer-supplied
// Joined/Gap own whitespace semantics; core/text owns every grapheme boundary
// and column conversion. Fingerprints refuse incompatible width projections.
type transcriptTextMap struct {
	rows        []text.Row
	starts      []text.Position
	lineLayouts []*text.Layout
	logical     *text.Layout
	fingerprint [32]byte
}

func newTranscriptTextMap(rows []text.Row) *transcriptTextMap {
	if len(rows) == 0 {
		return nil
	}
	m := &transcriptTextMap{rows: make([]text.Row, len(rows)), starts: make([]text.Position, len(rows)), lineLayouts: make([]*text.Layout, len(rows))}
	var builders []*strings.Builder
	for i, row := range rows {
		row.Text = text.Printable(row.Text)
		row.Gap = text.Printable(row.Gap)
		m.rows[i] = row
		if i == 0 || !row.Joined {
			builders = append(builders, new(strings.Builder))
		} else {
			_, _ = builders[len(builders)-1].WriteString(row.Gap)
		}
		line := len(builders) - 1
		m.starts[i] = text.Position{Line: line, Byte: builders[line].Len()}
		_, _ = builders[line].WriteString(row.Text)
		m.lineLayouts[i] = text.NewLayout(text.LayoutConfig{Lines: []string{row.Text}, Width: max(text.Width(row.Text)+1, 1), Wrap: text.WrapNone})
	}
	lines := make([]string, len(builders))
	for i, builder := range builders {
		lines[i] = builder.String()
	}
	m.logical = text.NewLayout(text.LayoutConfig{Lines: lines, Wrap: text.WrapNone})
	m.fingerprint = sha256.Sum256([]byte(strings.Join(lines, "\n")))
	return m
}
func (m *transcriptTextMap) start() text.Position { return text.Position{} }
func (m *transcriptTextMap) end() text.Position   { return m.logical.Hit(image.Pt(0, m.logical.Len())) }

func (m *transcriptTextMap) hit(row, col int, nearest bool) (text.Range, bool) {
	if m == nil || row < 0 || row >= len(m.rows) {
		return text.Range{}, false
	}
	content := m.rows[row]
	width := text.Width(content.Text)
	if !nearest && (col < content.Offset || col >= content.Offset+width) {
		return text.Range{}, false
	}
	col = max(col-content.Offset, 0)
	point := m.lineLayouts[row].Hit(image.Pt(col, 0))
	start := m.starts[row]
	start.Byte += point.Byte
	end := start
	end.Byte = m.starts[row].Byte + text.NextCluster(content.Text, point.Byte)
	return text.Range{Start: start, End: end}, true
}

func (m *transcriptTextMap) caret(position text.Position) (image.Point, bool) {
	if m == nil {
		return image.Point{}, false
	}
	position = m.logical.Normalize(position)
	for i, start := range m.starts {
		end := start.Byte + len(m.rows[i].Text)
		if start.Line != position.Line || position.Byte < start.Byte || position.Byte > end {
			continue
		}
		if position.Byte == end && i+1 < len(m.rows) && m.starts[i+1].Line == position.Line && position.Affinity == text.Forward {
			continue
		}
		point := m.lineLayouts[i].Caret(text.Position{Byte: position.Byte - start.Byte, Affinity: position.Affinity})
		return image.Pt(m.rows[i].Offset+point.X, i), true
	}
	return image.Point{}, false
}

func (m *transcriptTextMap) rects(selected text.Range) []image.Rectangle {
	selected.Start = m.logical.Normalize(selected.Start)
	selected.End = m.logical.Normalize(selected.End)
	var out []image.Rectangle
	for i, start := range m.starts {
		if start.Line < selected.Start.Line || start.Line > selected.End.Line {
			continue
		}
		from, to := 0, len(m.rows[i].Text)
		if start.Line == selected.Start.Line {
			from = max(from, selected.Start.Byte-start.Byte)
		}
		if start.Line == selected.End.Line {
			to = min(to, selected.End.Byte-start.Byte)
		}
		if to <= from {
			continue
		}
		for _, rect := range m.lineLayouts[i].Rects(text.Range{Start: text.Position{Byte: from}, End: text.Position{Byte: to}}) {
			out = append(out, rect.Add(image.Pt(m.rows[i].Offset, i)))
		}
	}
	return out
}

func (l TranscriptLayout) placed(id BlockID) (placed, bool) {
	for _, block := range l.blocks {
		if block.id == id {
			return block, true
		}
	}
	return placed{}, false
}

func (l TranscriptLayout) point(block placed, position text.Position) TextPoint {
	return TextPoint{Content: block.id, Revision: block.revision, Position: position, projection: block.mapping.fingerprint}
}

func (l TranscriptLayout) valid(point TextPoint) bool {
	block, ok := l.placed(point.Content)
	return ok && block.mapping != nil && block.revision == point.Revision && block.mapping.fingerprint == point.projection
}

// Hit maps a visible cell to a half-open logical range in its content revision.
func (l TranscriptLayout) Hit(point Point) (TextRange, bool) { return l.hit(point, false) }

func (l TranscriptLayout) hit(point Point, nearest bool) (TextRange, bool) {
	id, row, ok := l.At(point.Row)
	if !ok {
		return TextRange{}, false
	}
	block, _ := l.placed(id)
	span, ok := block.mapping.hit(row, point.Col, nearest)
	if !ok {
		return TextRange{}, false
	}
	return TextRange{Start: l.point(block, span.Start), End: l.point(block, span.End)}, true
}

// Caret maps a valid logical endpoint into this layout, refusing a changed text projection.
func (l TranscriptLayout) Caret(point TextPoint) (Point, bool) {
	if !l.valid(point) {
		return Point{}, false
	}
	block, _ := l.placed(point.Content)
	at, ok := block.mapping.caret(point.Position)
	return Point{Row: block.top + at.Y, Col: at.X}, ok
}

// Layout returns the last committed immutable geometry and logical-text mapping.
func (t *Transcript) Layout() TranscriptLayout { return t.geometry.Value() }

// SelectWord selects the logical word at a projected cell using the shared text layout.
func (s *Selection) SelectWord(layout TranscriptLayout, point Point) bool {
	selected, ok := layout.Hit(point)
	if !ok {
		return false
	}
	block, _ := layout.placed(selected.Start.Content)
	line, ok := block.mapping.logical.Row(selected.Start.Position.Line)
	if !ok {
		return false
	}
	start, end, ok := text.WordAt(line.Text, selected.Start.Position.Byte)
	if !ok {
		return false
	}
	selected.Start.Position.Byte, selected.End.Position.Byte = start, end
	s.Set(selected)
	return true
}

// SelectLine selects the displayed row as logical endpoints, preserving the source revision.
func (s *Selection) SelectLine(layout TranscriptLayout, point Point) bool {
	id, row, ok := layout.At(point.Row)
	if !ok {
		return false
	}
	block, _ := layout.placed(id)
	if block.mapping == nil || row >= len(block.mapping.rows) {
		return false
	}
	start := block.mapping.starts[row]
	end := start
	end.Byte += len(block.mapping.rows[row].Text)
	if end.Byte == start.Byte {
		return false
	}
	s.Set(TextRange{Start: layout.point(block, start), End: layout.point(block, end)})
	return true
}

// ReadingAnchor is a projection of reading intent into content coordinates, not another scroll owner.
type ReadingAnchor struct {
	Point          TextPoint
	ViewportOffset int
	FollowingEnd   bool
}

// ReadingAnchor is a projection of the scroll owner's current reading intent.
// It can be retained by a caller but cannot move the live viewport on its own.
func (t *Transcript) ReadingAnchor() (ReadingAnchor, bool) {
	current := t.geometry.Value()
	if t.scroll.FollowingEnd() {
		return ReadingAnchor{FollowingEnd: true}, true
	}
	id, row, ok := current.At(current.StartRow() + t.scroll.Offset())
	if !ok {
		return ReadingAnchor{}, false
	}
	block, _ := current.placed(id)
	if block.mapping == nil || row >= len(block.mapping.starts) {
		return ReadingAnchor{}, false
	}
	return ReadingAnchor{Point: current.point(block, block.mapping.starts[row])}, true
}
