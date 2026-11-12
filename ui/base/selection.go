package base

import (
	"errors"
	"image"
	"strings"
	"time"

	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/core/text"
)

// Point is a transient cell in a transcript layout, never selection ownership.
type Point struct{ Row, Col int }

// TextPoint addresses a grapheme boundary in one retained content revision.
// The private projection identity detects a renderer whose text projection changes
// meaning when its width changes; an old selection is refused, never reinterpreted.
type TextPoint struct {
	Content    BlockID
	Revision   uint64
	Position   text.Position
	identity   *byte // non-retaining identity: a BlockID is local to its transcript
	projection [32]byte
}

// Before compares logical positions in transcript order; it does not validate revision availability.
func (p TextPoint) Before(q TextPoint) bool {
	if p.Content != q.Content {
		return p.Content < q.Content
	}
	return p.Position.Line < q.Position.Line || p.Position.Line == q.Position.Line && p.Position.Byte < q.Position.Byte
}

// TextRange uses half-open logical endpoints; layout projection never rewrites its anchor.
type TextRange struct{ Start, End TextPoint }

// ErrStaleSelection reports that selected content or its logical text projection is no longer available.
var ErrStaleSelection = errors.New("base: selected content revision is unavailable")

// Selection owns logical anchors. Width, scroll and cell rectangles are read-only
// projections, so a resize cannot silently select a different word.
// Unknown position affinities panic before changing the anchors or gesture.
type Selection struct {
	element         scene.Element
	clicks          Clicks
	clickProjection selectionClickProjection
	anchor, extent  TextPoint
	phase           selectionPhase
}

// A repaint preserves a click run; reflow or scrolling can give the same screen
// coordinates another meaning even when the content revision stays unchanged.
type selectionClickProjection struct {
	layout transcriptLayoutKey
	origin image.Point
}

// selectionPhase is where the selection stands; only a selected range can be
// dragged, so a drag without one is not representable.
type selectionPhase uint8

const (
	selectionNone     selectionPhase = iota
	selectionHeld                    // a range is selected
	selectionDragging                // a range is selected and a drag is extending it
)

// Begin starts a directional selection anchored to one content revision.
// It panics for an unknown affinity or during a read-only projection, before changing the selection.
func (s *Selection) Begin(p TextPoint) {
	if s.setRange(TextRange{Start: p, End: p}, true) {
		s.element.Invalidate()
	}
}

// Extend moves the active endpoint while preserving the gesture anchor.
// It panics for an unknown affinity even without an active drag, or during a read-only projection.
// Refusal leaves the selection unchanged.
func (s *Selection) Extend(p TextPoint) {
	requireMutable(s.element)
	if err := p.Position.Affinity.Validate(); err != nil {
		panic(err)
	}

	if s.phase == selectionDragging && s.extent != p {
		s.extent = p
		s.element.Invalidate()
	}
}

func (s *Selection) Anchor() TextPoint { return s.anchor }

// Done ends dragging while retaining the selected logical range.
func (s *Selection) Done() {
	requireMutable(s.element)
	if s.phase == selectionDragging {
		s.phase = selectionHeld
		s.element.Invalidate()
	}
}

func (s *Selection) press(mouse input.Mouse, source transcriptLayoutKey, at Point) int {
	requireMutable(s.element)
	projection := selectionClickProjection{layout: source, origin: mouse.Pos.Sub(image.Pt(at.Col, at.Row))}
	if s.clickProjection != projection {
		s.clicks.Reset()
		s.clickProjection = projection
	}
	return s.clicks.Press(mouse)
}

// cancel ends the gesture together with its click run.
func (s *Selection) cancel() {
	requireMutable(s.element)
	s.clicks.Reset()
	s.Done()
}

func (s *Selection) Clear() {
	if s.clear() {
		s.element.Invalidate()
	}
}

func (s *Selection) clear() bool {
	requireMutable(s.element)
	changed := s.phase != selectionNone || s.anchor != (TextPoint{}) || s.extent != (TextPoint{})
	s.anchor, s.extent = TextPoint{}, TextPoint{}
	s.phase = selectionNone
	s.clicks.Reset()
	return changed
}

// SetClickInterval configures the gap between presses in one selection gesture.
// Nonpositive values use DefaultMultiClick. It panics during a read-only projection.
func (s *Selection) SetClickInterval(interval time.Duration) {
	requireMutable(s.element)
	s.clicks.Within = interval
}

// Active reports whether a logical range is selected, including an empty range.
func (s *Selection) Active() bool { return s.phase != selectionNone }

func (s *Selection) Dragging() bool { return s.phase == selectionDragging }

// Range returns endpoints in reading order without changing the directional anchor.
func (s *Selection) Range() (TextPoint, TextPoint) {
	if s.extent.Before(s.anchor) {
		return s.extent, s.anchor
	}
	return s.anchor, s.extent
}

// Set replaces the logical range and settles any active drag.
// It panics for an unknown endpoint affinity or during a read-only projection, before changing the selection.
func (s *Selection) Set(selected TextRange) {
	if s.setRange(selected, false) {
		s.element.Invalidate()
	}
}

func (s *Selection) setRange(selected TextRange, dragging bool) bool {
	requireMutable(s.element)
	for _, point := range [...]TextPoint{selected.Start, selected.End} {
		if err := point.Position.Affinity.Validate(); err != nil {
			panic(err)
		}
	}
	phase := selectionHeld
	if dragging {
		phase = selectionDragging
	}
	changed := s.phase != phase || s.anchor != selected.Start || s.extent != selected.End
	s.anchor, s.extent, s.phase = selected.Start, selected.End, phase
	return changed
}

// contentChanged clears a range touching replaced semantic content. There is
// no guessed byte adjustment across arbitrary renderer revisions. Only the
// transcript that owns the content identities retires them.
func (s *Selection) contentChanged(id BlockID, revision uint64) bool {
	requireMutable(s.element)

	if s.phase == selectionNone {
		return false
	}
	first, last := s.Range()
	if id >= first.Content && id <= last.Content && (id != first.Content || first.Revision != revision || id != last.Content || last.Revision != revision) {
		return s.clear()
	}
	return false
}

// discardBefore clears a selection that touches retired content identities.
func (s *Selection) discardBefore(id BlockID) bool {
	requireMutable(s.element)

	if s.phase == selectionNone {
		return false
	}
	first, _ := s.Range()
	if first.Content < id {
		return s.clear()
	}
	return false
}

// Text copies safe logical text from the supplied projection, joining soft wraps without inventing source syntax.
func (s *Selection) Text(layout TranscriptLayout) (string, error) {
	if s.phase == selectionNone {
		return "", nil
	}
	first, last := s.Range()
	if !layout.valid(first) || !layout.valid(last) {
		return "", ErrStaleSelection
	}
	var out strings.Builder
	wrote := false
	for _, block := range layout.blocks {
		if block.id < first.Content || block.id > last.Content || block.mapping == nil {
			continue
		}
		from, to := block.mapping.start(), block.mapping.end()
		if block.id == first.Content {
			from = first.Position
		}
		if block.id == last.Content {
			to = last.Position
		}
		if wrote {
			out.WriteByte('\n')
		}
		wrote = true
		out.WriteString(block.mapping.logical.Text(text.Range{Start: from, End: to}))
	}
	return out.String(), nil
}

// Rects projects the logical range into cells for this layout and refuses stale interpretations.
func (s *Selection) Rects(layout TranscriptLayout) ([]image.Rectangle, error) {
	if s.phase == selectionNone {
		return nil, nil
	}
	first, last := s.Range()
	if !layout.valid(first) || !layout.valid(last) {
		return nil, ErrStaleSelection
	}
	var rects []image.Rectangle
	for _, block := range layout.blocks {
		if block.id < first.Content || block.id > last.Content || block.mapping == nil {
			continue
		}
		from, to := block.mapping.start(), block.mapping.end()
		if block.id == first.Content {
			from = first.Position
		}
		if block.id == last.Content {
			to = last.Position
		}
		for _, rect := range block.mapping.rects(text.Range{Start: from, End: to}) {
			rects = append(rects, rect.Add(image.Pt(0, block.top)))
		}
	}
	return rects, nil
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
// A terminal reports two presses and not a double-click, so whether they are one
// gesture is a question about when they arrived. The time is an argument rather than
// read here: a type that called time.Now could not be told to be at a particular
// moment.
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
func (c *Clicks) Reset() {
	c.at, c.last, c.count = image.Point{}, time.Time{}, 0
}

func coordinatesNear(a, b, slack int) bool {
	if slack < 0 {
		return false
	}
	if a < b {
		a, b = b, a
	}
	return uint(a)-uint(b) <= uint(slack)
}
