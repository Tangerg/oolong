package grid

import (
	"image"
	"io"
)

// CursorShape is the terminal cursor's geometry.
type CursorShape uint8

const (
	// CursorDefault asks the terminal for its configured default shape. Blink is
	// ignored. It is the zero value and the shape restored when a session ends.
	CursorDefault CursorShape = iota
	// CursorBlock fills one cell.
	CursorBlock
	// CursorUnderline is a line along the bottom of one cell.
	CursorUnderline
	// CursorBar is a vertical line inside one cell.
	CursorBar
)

// CursorStyle is how the terminal's own cursor is drawn. The zero value uses the
// terminal's default. Blink applies to Block, Underline and Bar.
type CursorStyle struct {
	Shape CursorShape
	Blink bool
}

func (s CursorStyle) sequence() string {
	switch s.Shape {
	case CursorBlock:
		if s.Blink {
			return blinkingBlockCursor
		}
		return steadyBlockCursor
	case CursorUnderline:
		if s.Blink {
			return blinkingUnderlineCursor
		}
		return steadyUnderlineCursor
	case CursorBar:
		if s.Blink {
			return blinkingBarCursor
		}
		return steadyBarCursor
	default:
		return defaultCursor
	}
}

func (s CursorStyle) normalized() CursorStyle {
	if s.Shape > CursorBar {
		return CursorStyle{}
	}
	if s.Shape == CursorDefault {
		s.Blink = false
	}
	return s
}

// Cursor is where and how the terminal's own cursor should end a frame.
type Cursor struct {
	Visible bool
	Pos     image.Point
	Style   CursorStyle
}

// Screen owns double-buffered cells and the terminal publication model. Unchanged
// frames write nothing so cursor blinking is undisturbed. A Screen must not be copied
// after first use and belongs to one goroutine. Flush callbacks may read the canvas
// but cannot mutate it or retained frame views; reentrant mutations panic.
type Screen struct {
	noCopy      noCopy
	publication publication

	buffers frameBuffer
	cursor  cursorState
	placed  Cursor

	// frame and scratch are reused across flushes. scratch measures a scroll
	// candidate before committing to it.
	frame   painter
	scratch painter
	// out is the single buffer handed to the writer, so one frame is one write.
	out []byte

	// full forces the next flush to repaint every cell, because the terminal's
	// contents are no longer known: after a resize, or after something else has
	// written to the terminal.
	full bool
	// repaintAll says the same about the regions something else painted, which the
	// cells say nothing about. It is a second flag because the cell repaint happens
	// before them and clears the first one.
	repaintAll bool
}

// NewScreen returns a screen of the given size whose first flush repaints
// everything.
func NewScreen(w, h int) *Screen {
	return &Screen{
		buffers: newFrameBuffer(w, h),
		full:    true,
	}
}

// Size returns the screen's width and height.
func (s *Screen) Size() (w, h int) { return s.buffers.size() }

// Resize changes the screen's size. The next flush repaints everything: after a
// resize the terminal has reflowed its own contents, and nothing about what it is
// showing can be assumed.
func (s *Screen) Resize(w, h int) {
	s.publication.requireIdle()
	if s.buffers.resize(w, h) {
		s.Invalidate()
	}
}

// SetDepth says how much colour the terminal can show. It forces a full repaint,
// because every cell the terminal is holding was encoded at the old depth.
func (s *Screen) SetDepth(d Depth) {
	s.publication.requireIdle()
	if !s.buffers.setDepth(d) {
		return
	}
	s.frame.adoptDepth(d)
	s.scratch.adoptDepth(d)
	s.Invalidate()
}

// SetGround says what the terminal's own two colours are, so that a layer drawn
// over another can mix with it — see [View.Blend].
//
// Both surfaces are told, because a flush swaps them. No repaint is forced: the
// ground is read while a frame is being drawn, so the next frame uses it by drawing
// itself, and nothing already on the terminal was encoded with it.
func (s *Screen) SetGround(g Ground) {
	s.publication.requireIdle()
	s.buffers.setGround(g)
}

// Invalidate forgets what the terminal is showing, so the next flush repaints in
// full. It is what to call after handing the terminal to another program.
func (s *Screen) Invalidate() {
	s.publication.requireIdle()
	s.invalidate()
}

func (s *Screen) invalidate() {
	s.full = true
	s.repaintAll = true
	s.cursor.forget()
}

// Frame blanks the drawing surface and returns the view for this frame.
//
// Every frame draws everything it wants to be visible. Keeping content across
// frames is the diff's job, not the caller's, and a surface that carried
// yesterday's cells forward would make a missed redraw look like success.
func (s *Screen) Frame() View {
	s.publication.requireIdle()
	return s.buffers.begin(&s.placed)
}

// Cursor is where the last frame asked for the terminal's cursor to go.
//
// It reads back what [View.PlaceCursor] recorded, which is otherwise only
// observable by decoding the escape stream. A caller that has to know where the
// caret is — to place something beside it, or to check that drawing put it where
// it meant to — had no way to ask before this.
func (s *Screen) Cursor() Cursor { return s.placed }

// Flush writes this frame to w, leaving the cursor wherever the frame placed it.
// A destination write failure permanently ends this publication session: a prefix
// may have changed cells, placements or terminal parsing state. Later Flush calls
// return the original error without writing. Invalidate cannot recover that session.
// An abnormal writer exit also ends the session; a recovered panic cannot authorize
// replaying an unknown partial write.
func (s *Screen) Flush(w io.Writer) error {
	s.publication.begin()
	defer s.publication.end()
	if s.publication.failure != nil {
		return s.publication.failure
	}
	composed := false
	defer func() {
		if !composed {
			s.invalidate()
		}
	}()
	s.frame.restart()
	s.paintCells()
	s.frame.end()

	// The regions something else paints go after the cells and before the cursor: the
	// diff would otherwise write the blanks it thinks are underneath them over what
	// was painted, and the cursor has to end up where the frame asked whatever a
	// painter did with it.
	if err := s.paintRegions(); err != nil {
		return err
	}
	// A frame that began is a frame that wrote cells, which is also what tells the
	// cursor it has to re-anchor.
	s.cursor.emit(&s.frame, s.placed, len(s.frame.out) > 0)
	composed = true

	if len(s.frame.out) == 0 {
		s.buffers.swap()
		return nil
	}
	s.out = append(s.out[:0], beginSync...)
	s.out = append(s.out, s.frame.out...)
	s.out = append(s.out, endSync...)
	if err := s.publication.write(w, s.out); err != nil {
		return err
	}
	s.buffers.swap()
	return nil
}

// paintRegions writes what turns the regions the terminal is showing into the ones
// this frame asked for.
//
// A full repaint says nothing about them: what the terminal remembers being shown is
// not a cell and is not undone by writing over the cells. So a frame that repainted
// everything erases every region it knew about and paints them again, which is also
// what makes a resize and a handover come back right.
func (s *Screen) paintRegions() error {
	was := s.buffers.front.regions()
	out := bytesTo{&s.frame.out}
	if s.repaintAll {
		s.repaintAll = false
		if err := repaint(out, was, nil, s.frame.moveToPoint); err != nil {
			return err
		}
		was = nil
	}
	if err := repaint(out, was, s.buffers.back.regions(), s.frame.moveToPoint); err != nil {
		return err
	}
	if len(s.buffers.back.regions()) > 0 || len(was) > 0 {
		// A painter was handed the writer, and what it wrote is not something this
		// package can read: where the cursor ended up is no longer known.
		s.frame.forcePos()
	}
	return nil
}

func (s *Screen) paintCells() {
	if s.full {
		s.full = false
		s.frame.paint(s.buffers.back, 0, s.buffers.back.h, everything)
		return
	}
	if s.paintScroll() {
		return
	}
	s.frame.paint(s.buffers.back, 0, s.buffers.back.h, changedAgainst(s.buffers.front, s.buffers.back))
}

// paintScroll takes the terminal's own scrolling shortcut when the frame is a row
// shift and the shortcut is genuinely shorter.
//
// A shift is only worth taking if it beats the diff it replaces, so each
// candidate is measured against a floor on what the diff must cost. Measuring
// against a floor rather than against the diff itself means the diff is never
// built twice.
func (s *Screen) paintScroll() bool {
	// Terminal scrolling moves placements too; use ordinary diff while images exist.
	if len(s.buffers.front.paints) > 0 || len(s.buffers.back.paints) > 0 {
		return false
	}
	shifts := detectShifts(s.buffers.front, s.buffers.back)
	if len(shifts) == 0 {
		return false
	}
	floor := diffCostFloor(s.buffers.front, s.buffers.back)
	if floor == 0 {
		return false
	}

	best := -1
	bestLen := 0
	for i, shift := range shifts {
		s.scratch.restart()
		s.scratch.scroll(s.buffers.back, shift)
		n := len(s.scratch.out)
		if n < floor && (best < 0 || n < bestLen) {
			best, bestLen = i, n
		}
	}
	if best < 0 {
		return false
	}
	s.scratch.restart()
	s.scratch.scroll(s.buffers.back, shifts[best])
	s.frame.out = append(s.frame.out, s.scratch.out...)
	// The scratch painter established the style and cursor state that the frame
	// painter must now continue from.
	s.frame.adopt(&s.scratch)
	return true
}

// cursorState de-duplicates cursor commands across frames.
//
// The point is the blink timer: a terminal restarts it on every positioning
// command, so a UI that re-states an unchanged cursor position every frame has a
// cursor that never blinks. An idle frame must therefore say nothing at all, and
// a frame that only wrote cells must re-anchor without moving.
type cursorState struct {
	known      bool
	visible    bool
	pos        image.Point
	styleKnown bool
	style      CursorStyle
}

func (c *cursorState) forget() { *c = cursorState{} }

func (c *cursorState) emit(p *painter, next Cursor, cellsChanged bool) {
	defer func() {
		c.known = true
		c.visible = next.Visible
		if next.Visible {
			c.pos = next.Pos
			c.styleKnown = true
			c.style = next.Style.normalized()
		}
	}()

	if next.Visible {
		style := next.Style.normalized()
		if !c.styleKnown || style != c.style {
			p.out = append(p.out, style.sequence()...)
		}
	}
	if !c.known {
		if next.Visible {
			p.moveTo(next.Pos.X, next.Pos.Y)
			p.out = append(p.out, showCursor...)
			return
		}
		p.out = append(p.out, hideCursor...)
		return
	}
	switch {
	case next.Visible && !c.visible:
		p.moveTo(next.Pos.X, next.Pos.Y)
		p.out = append(p.out, showCursor...)
	case !next.Visible && c.visible:
		p.out = append(p.out, hideCursor...)
	case next.Visible && (next.Pos != c.pos || cellsChanged):
		// Writing cells left the terminal's cursor wherever the last glyph
		// landed, so an unmoved cursor still has to be re-anchored.
		p.forcePos()
		p.moveTo(next.Pos.X, next.Pos.Y)
	}
}

const (
	showCursor              = "\x1b[?25h"
	hideCursor              = "\x1b[?25l"
	defaultCursor           = "\x1b[0 q"
	blinkingBlockCursor     = "\x1b[1 q"
	steadyBlockCursor       = "\x1b[2 q"
	blinkingUnderlineCursor = "\x1b[3 q"
	steadyUnderlineCursor   = "\x1b[4 q"
	blinkingBarCursor       = "\x1b[5 q"
	steadyBarCursor         = "\x1b[6 q"
)
