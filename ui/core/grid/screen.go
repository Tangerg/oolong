package grid

import (
	"image"
	"io"
)

type CursorShape uint8

const (
	// CursorDefault asks the terminal for its configured default shape. Blink is
	// ignored. It is the zero value and the shape restored when a session ends.
	CursorDefault CursorShape = iota
	CursorBlock
	CursorUnderline
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

	// full forces the next flush to repaint the composition, because the terminal's
	// contents are no longer known: after a resize, or after something else has
	// written to the terminal.
	full bool
}

func NewScreen(w, h int) *Screen { return (NativeTarget{}).Screen(w, h) }

// Screen creates a canvas fixed to the native destination for its lifetime.
func (target NativeTarget) Screen(w, h int) *Screen {
	return &Screen{
		buffers: newFrameBuffer(w, h, target),
		full:    true,
	}
}

// Size measures canvas cells, independently of native image pixel dimensions.
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
// An invalid depth is a programmer error and panics before changing the canvas.
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
	s.cursor.forget()
}

// Frame blanks the drawing surface and returns the view for this frame.
//
// Every frame draws everything it wants to be visible. Keeping content across
// frames is the diff's job, not the caller's, and a surface that carried
// yesterday's cells forward would make a missed redraw look like success.
// The view and its subviews may be written until the next Frame, a size change,
// or Flush begins. Writes after that boundary panic.
func (s *Screen) Frame() View {
	s.publication.requireIdle()
	return s.buffers.begin(&s.placed, &s.publication)
}

// Cursor is the frame's requested cursor; it can differ from the terminal until
// the frame's output commits.
func (s *Screen) Cursor() Cursor { return s.placed }

// Prepare encodes a candidate frame without changing the accepted display.
func (s *Screen) Prepare() (output *Output, err error) {
	s.publication.begin()
	s.buffers.back.endDrawing()
	beforeCursor := s.cursor
	defer func() {
		s.cursor = beforeCursor
		if output == nil {
			s.publication.end()
		}
	}()
	if s.publication.failure != nil {
		return nil, s.publication.failure
	}
	if err = s.buffers.checkTargets(); err != nil {
		return nil, err
	}
	full := s.full || s.buffers.regionsNeedRepaint()
	s.frame.restart()
	if full {
		if err := eraseRegions(s.buffers.target, &s.frame.out, s.buffers.front.regions()); err != nil {
			return nil, err
		}
	}
	s.paintCells(full)
	s.frame.end()
	if full {
		if err := s.paintRegions(); err != nil {
			return nil, err
		}
	}
	s.cursor.emit(&s.frame, s.placed, len(s.frame.out) > 0)
	afterCursor := s.cursor
	var payload []byte
	if len(s.frame.out) > 0 {
		payload = append(payload, beginSync...)
		payload = append(payload, s.frame.out...)
		payload = append(payload, endSync...)
	}
	output = newOutput(payload, func(commit bool) {
		if commit {
			s.cursor, s.full = afterCursor, false
			s.buffers.swap()
		}
		s.publication.end()
	})
	return output, nil
}

// Flush synchronously writes one prepared transaction. Any partial write makes
// this output session unusable; admission-based runtimes use Prepare instead.
func (s *Screen) Flush(w io.Writer) error {
	output, err := s.Prepare()
	if err != nil {
		return err
	}
	defer output.Abort()
	if output.Len() > 0 {
		if err := s.publication.write(w, output.payload); err != nil {
			return err
		}
	}
	output.Commit()
	return nil
}

// Opaque painters go after the restored cells, in their registered drawing order.
func (s *Screen) paintRegions() error {
	if err := paintRegions(s.buffers.target, &s.frame.out, s.buffers.back.regions(), s.frame.moveToPoint); err != nil {
		return err
	}
	if len(s.buffers.back.regions()) > 0 {
		// Opaque output cannot update the encoder's cursor cache, so later
		// cursor restoration must position explicitly.
		s.frame.forcePos()
	}
	return nil
}

func (s *Screen) paintCells(full bool) {
	if full {
		s.frame.paint(s.buffers.back, 0, s.buffers.back.h, everything)
		return
	}
	if s.paintScroll() {
		return
	}
	s.frame.paint(s.buffers.back, 0, s.buffers.back.h, changedAgainst(s.buffers.front, s.buffers.back))
}

// Compare scroll candidates against a lower bound on ordinary diff cost.
// A winning candidate is shorter without encoding the rejected diff twice.
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

// cursorState avoids redundant positioning because each command can restart
// terminal blinking. Cell output still requires re-anchoring the requested
// cursor; a completely idle frame emits nothing.
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
