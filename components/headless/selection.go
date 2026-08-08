package headless

import (
	"image"
	"strings"
	"time"

	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/layout"
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
	// Clicks counts the run a press belongs to, which is what tells a double-click
	// from two clicks. It lives here because a selection is the only thing that asks:
	// a second press takes the word and a third takes the line, and both are
	// questions about this selection rather than about the pointer in general.
	//
	// It is a field rather than something the caller keeps and passes in, because a
	// widget that had to be handed the state of its own gesture is a widget its
	// caller has to understand to use.
	Clicks Clicks

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

// DiscardBefore removes the part of a selection whose rows no longer belong to the
// transcript. A selection wholly before row is cleared; one crossing row begins at
// the first cell that remains.
func (s *Selection) DiscardBefore(row int) {
	if !s.active {
		return
	}
	start, end := s.Range()
	if end.Row < row {
		s.Clear()
		return
	}
	if start.Row >= row {
		return
	}
	if s.anchor.Row < row {
		s.anchor = Point{Row: row}
	}
	if s.extent.Row < row {
		s.extent = Point{Row: row}
	}
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
// hard-wrapped at whatever the window happened to be. See [text.Row].
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
	firstRow := max(start.Row, t.StartRow())
	lastRow := min(end.Row, layout.Remaining(t.EndRow(), 1))
	if firstRow > lastRow {
		return ""
	}
	rows := t.Rows(firstRow, layout.Sum(layout.Remaining(lastRow, firstRow), 1))
	if len(rows) == 0 {
		return ""
	}

	var b strings.Builder
	for i, row := range rows {
		if i > 0 {
			b.WriteString(row.Separator())
		}
		from, to := 0, len(row.Text)
		rowAt := layout.Sum(firstRow, i)
		if rowAt == start.Row {
			from = clusterAtOrAfter(row.Text, layout.Remaining(start.Col, row.Offset))
		}
		if rowAt == end.Row {
			endExclusive := 0
			if end.Col >= 0 {
				endExclusive = layout.Sum(end.Col, 1)
			}
			to = text.OffsetAt(row.Text, layout.Remaining(endExclusive, row.Offset))
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

// DefaultMultiClick is how close together two presses have to be to count as one
// gesture. It is what desktop systems have settled on.
const DefaultMultiClick = 400 * time.Millisecond

// multiClickSlack is how far the pointer may move between presses of one gesture. A
// cell, because a hand on a trackpad does not hold a pixel and a gesture that broke
// because the pointer drifted one column would feel like the double-click had failed.
const multiClickSlack = 1

// Clicks counts a run of presses in the same place as one gesture: one for a single
// click, two for a double, three for a triple.
//
// # Why this is not somewhere else
//
// A terminal does not report a double-click. It reports two presses, and whether they
// are one gesture is a question about when they arrived — so it can only be answered
// by whatever has a clock.
//
// The time is an argument rather than read here, which is the same bargain the rest of
// this library makes: a type that called time.Now could not be told to be at a
// particular moment, and every test of it would be a test of how fast the machine ran.
type Clicks struct {
	// Within is how close together presses must be. Zero uses [DefaultMultiClick].
	Within time.Duration

	at    image.Point
	last  time.Time
	count int
}

// Press records a press and reports which of the run it is.
//
// A press far from the last one, or long after it, starts a new run — and so does the
// first press of all, because the zero value has never seen one, and so does one that
// arrived with no time on it, because there is nothing to compare.
//
// The time comes from the event rather than from a clock here, because arrival is a
// fact about the input and the thing that read it is the only thing that knows.
func (c *Clicks) Press(ev input.Mouse) int {
	return c.press(ev.Pos, ev.At)
}

func (c *Clicks) press(at image.Point, when time.Time) int {
	if when.IsZero() {
		c.at, c.last, c.count = at, when, 1
		return 1
	}
	within := c.Within
	if within <= 0 {
		within = DefaultMultiClick
	}
	near := coordinatesNear(at.X, c.at.X, multiClickSlack) &&
		coordinatesNear(at.Y, c.at.Y, multiClickSlack)
	elapsed := when.Sub(c.last)
	if c.count == 0 || !near || elapsed < 0 || elapsed > within {
		c.count = 1
	} else {
		c.count++
	}
	c.at, c.last = at, when
	return c.count
}

// Reset forgets the run, which a caller does when something else has happened that
// makes the next press a first one.
func (c *Clicks) Reset() { c.count = 0 }

func coordinatesNear(a, b, slack int) bool {
	if slack < 0 {
		return false
	}
	if a < b {
		a, b = b, a
	}
	return uint(a)-uint(b) <= uint(slack)
}

// SelectWord selects the word at a point, which is what a double-click means.
//
// Where a word begins and ends is [text.WordAt]'s to say — including the part that
// matters for text written without spaces, where the run of one script is the word.
// It reports false when there is no word there, so a double-click in the margin
// selects nothing rather than selecting the gap.
func (s *Selection) SelectWord(t *Transcript, p Point) bool {
	row, ok := rowText(t, p.Row)
	if !ok || p.Col < row.Offset {
		// A negative column is not a position in the text, which is what [Selection.Covers]
		// already says about one. Clamping it to the start instead would make a click
		// in the margin select the first word.
		return false
	}
	at := text.OffsetAt(row.Text, p.Col-row.Offset)
	start, end, ok := text.WordAt(row.Text, at)
	if !ok {
		return false
	}
	s.set(p.Row,
		layout.Sum(row.Offset, text.ColumnOf(row.Text, start)),
		layout.Remaining(layout.Sum(row.Offset, text.ColumnOf(row.Text, end)), 1),
	)
	return true
}

// SelectLine selects a whole row, which is what a triple-click means.
//
// The row, not the logical line the width broke it out of. What a triple-click selects
// is what the reader sees as a line, and asking for the line behind it would take text
// they can neither see nor point at.
func (s *Selection) SelectLine(t *Transcript, p Point) bool {
	row, ok := rowText(t, p.Row)
	if !ok {
		return false
	}
	width := text.Width(row.Text)
	if width == 0 {
		return false
	}
	s.set(p.Row, max(row.Offset, 0), layout.Remaining(layout.Sum(row.Offset, width), 1))
	return true
}

// set puts the selection over one row's columns, both ends inclusive, and leaves no
// drag in progress: a click that selected something has finished selecting it.
func (s *Selection) set(row, from, to int) {
	s.anchor = Point{Row: row, Col: from}
	s.extent = Point{Row: row, Col: to}
	s.active, s.dragging = true, false
}

// rowText is the text of one row of a transcript, and whether there is one.
func rowText(t *Transcript, row int) (text.Row, bool) {
	if t == nil {
		return text.Row{}, false
	}
	rows := t.Rows(row, 1)
	if len(rows) == 0 {
		return text.Row{}, false
	}
	return rows[0], true
}
