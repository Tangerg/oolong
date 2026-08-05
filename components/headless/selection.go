package headless

import (
	"strings"

	"github.com/Tangerg/oolong/core/text"
)

// Point addresses a cell of a transcript: a row in its coordinate space, and a
// column across.
//
// The row is absolute rather than a position on screen, which is the whole reason
// the transcript numbers its rows. A selection made and then scrolled past is still
// over the same words; one held in screen coordinates would slide up the text as the
// view moved, which is not what anybody dragged.
type Point struct{ Row, Col int }

// Before reports whether p comes earlier in the text than q.
func (p Point) Before(q Point) bool {
	return p.Row < q.Row || (p.Row == q.Row && p.Col < q.Col)
}

// Selection is a range of a transcript the user has dragged over.
//
// The two ends are an anchor, where the drag began, and an extent, where it is now.
// They are kept in that order rather than sorted, because a drag upwards is a real
// thing and the anchor has to stay where the user put it: sorting on the way in would
// make the selection turn inside out as the pointer crossed its own start.
//
// The zero value selects nothing.
type Selection struct {
	anchor, extent Point
	active         bool
	dragging       bool
}

// Begin starts a selection at p and marks a drag in progress.
func (s *Selection) Begin(p Point) {
	s.anchor, s.extent = p, p
	s.active, s.dragging = true, true
}

// Extend moves the far end to p. It does nothing unless a drag is in progress, so a
// pointer moving over the text without a button held changes nothing.
func (s *Selection) Extend(p Point) {
	if !s.dragging {
		return
	}
	s.extent = p
}

// Done ends the drag, leaving the selection where it is.
func (s *Selection) Done() { s.dragging = false }

// Clear removes the selection.
func (s *Selection) Clear() {
	*s = Selection{}
}

// Active reports whether anything is selected.
func (s *Selection) Active() bool { return s.active }

// Dragging reports whether the pointer is still down and the far end still moving.
func (s *Selection) Dragging() bool { return s.dragging }

// Range is the selection in reading order: the earlier end first.
//
// Both ends are inclusive, which is what a drag means — a pointer dragged over a
// character has selected that character, and an exclusive end would leave the one
// under the pointer out.
func (s *Selection) Range() (start, end Point) {
	if s.extent.Before(s.anchor) {
		return s.extent, s.anchor
	}
	return s.anchor, s.extent
}

// Empty reports whether the selection covers no cells at all.
func (s *Selection) Empty() bool { return !s.active }

// Covers reports whether a cell is inside the selection, which is what painting the
// highlight asks once per cell.
func (s *Selection) Covers(row, col int) bool {
	if !s.active {
		return false
	}
	start, end := s.Range()
	switch {
	case row < start.Row || row > end.Row:
		return false
	case row == start.Row && col < start.Col:
		return false
	case row == end.Row && col > end.Col:
		return false
	default:
		return true
	}
}

// Text is what the selection would put on the clipboard.
//
// Rows are joined the way the text was written rather than the way it was laid out.
// A row the width made is rejoined to the one above it with whatever the wrap
// consumed at that break, and a row the text made begins a new line — so a paragraph
// copied out of a narrow window pastes as a paragraph, not as a column of fragments
// hard-wrapped at whatever the window happened to be. See [Row].
//
// Columns are sliced on cluster boundaries. A wide character is taken only when it
// lies wholly inside the selection: half of one cannot be put on a clipboard, and
// including a character the user only touched the edge of is the error that is
// noticed, because it is the one that appears at the ends of every copy.
func (s *Selection) Text(t *Transcript) string {
	if !s.active || t == nil {
		return ""
	}
	start, end := s.Range()
	rows := t.Rows(start.Row, end.Row-start.Row+1)
	if len(rows) == 0 {
		return ""
	}

	var b strings.Builder
	for i, row := range rows {
		if i > 0 {
			b.WriteString(row.Separator())
		}
		from, to := 0, len(row.Text)
		if i == 0 {
			from = clusterAtOrAfter(row.Text, start.Col)
		}
		if i == len(rows)-1 {
			to = text.OffsetAt(row.Text, end.Col+1)
		}
		if from < to {
			b.WriteString(strings.TrimRight(row.Text[from:to], " "))
		}
	}
	return b.String()
}

// clusterAtOrAfter is the byte offset of the first cluster that begins at or after a
// column.
//
// A wide character straddling the column began before it, so it is not wholly inside
// the selection and the offset moves past it. The far end applies the same rule from
// the other side, which is what keeps a selection's two edges consistent: whether a
// character is in or out does not depend on which end of the drag it was at.
func clusterAtOrAfter(s string, col int) int {
	at := text.OffsetAt(s, col)
	if text.ColumnOf(s, at) < col {
		return text.NextCluster(s, at)
	}
	return at
}
