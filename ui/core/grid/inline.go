package grid

import (
	"errors"
	"fmt"
	"image"
	"io"
)

const eraseLine = "\x1b[K"

// Inline keeps finished output in terminal scrollback and draws the active block
// relative to its last cursor anchor. Autowrap must be enabled so live rows form
// one soft-wrapped logical line. Resizing requires the terminal to preserve the
// cursor's logical line, as xterm.js does by default, or to leave all rows
// un-reflowed. A full repaint restores truncated cells.
// Rows pushed into scrollback by a height smaller than the live block cannot be
// recovered without owning the terminal's history.
//
// An Inline belongs to one goroutine and must not be copied after first use.
// Prepare and synchronous publication callbacks may read the canvas but cannot
// mutate retained frame views or reenter mutating Inline methods; violations panic.
type Inline struct {
	noCopy      noCopy
	publication publication

	buffers frameBuffer

	placed Cursor

	// contribution is private candidate data, never an admission queue.
	contribution [][]Cell

	// Only a committed output advances the anchor. A failed synchronous write
	// makes relative placement unknowable and permanently ends the session.
	rows int
	at   image.Point

	// Unchanged visibility writes nothing, preserving cursor blink.
	known bool
	shown bool
	// Visibility and shape are independent; hiding does not reset shape.
	cursorStyleKnown bool
	cursorStyle      CursorStyle

	// A full repaint restores cells and terminal-native regions as one composition.
	full bool

	// Completion belongs to Inline so exit and handover callers can finish
	// the same block without each retaining a competing completion flag.
	left bool

	buf []byte
}

// NewInline uses the terminal's current viewport dimensions. The live block
// occupies only rows drawn by the frame. Publication requires dimensions accepted
// by [ValidateTerminalSize]; changing content height does not resize the viewport.
func NewInline(w, h int) *Inline { return (NativeTarget{}).Inline(w, h) }

// Inline creates a canvas fixed to the native destination for its lifetime.
func (target NativeTarget) Inline(w, h int) *Inline {
	return &Inline{
		buffers: newFrameBuffer(w, h, target),
		full:    true,
	}
}

// Size returns the last supplied terminal viewport dimensions.
func (i *Inline) Size() (w, h int) { return i.buffers.size() }

// Resize reports the terminal's current viewport dimensions because Inline cannot
// observe terminal reflow itself. Changing the live block's height only requires a
// new frame. Prepare projects caller-owned frozen contributions at the current width.
func (i *Inline) Resize(w, h int) {
	i.publication.requireIdle()
	if i.buffers.resize(w, h) {
		i.Invalidate()
	}
}

// SetDepth says how much colour the terminal can show. It forces a full repaint,
// because every row the terminal is holding was encoded at the old depth.
// An invalid depth is a programmer error and panics before changing the canvas.
func (i *Inline) SetDepth(d Depth) {
	i.publication.requireIdle()
	if i.buffers.setDepth(d) {
		i.Invalidate()
	}
}

// SetGround says what the terminal's own two colours are, so that a layer drawn
// over another can mix with it — see [View.Blend].
func (i *Inline) SetGround(g Ground) {
	i.publication.requireIdle()
	i.buffers.setGround(g)
}

func (i *Inline) Invalidate() {
	i.publication.requireIdle()
	i.invalidate()
}

func (i *Inline) invalidate() {
	i.full = true
	i.known = false
	i.cursorStyleKnown = false
}

// Frame blanks the drawing surface and returns the view for this frame.
//
// The view is as tall as the block may grow to, not as tall as the block ends up:
// its height is decided by what this frame draws into it.
// The view and its subviews may be written until the next Frame, a size change,
// or Prepare or PrepareFinish begins, including through Flush or Finish.
// Writes outside that lifetime panic.
func (i *Inline) Frame() View {
	i.publication.requireIdle()
	return i.buffers.begin(&i.placed, &i.publication)
}

// Cursor is where the last frame asked for the terminal's cursor to go, in the
// block's own coordinates. See [Screen.Cursor].
func (i *Inline) Cursor() Cursor { return i.placed }

type inlineState struct {
	rows                           int
	at                             image.Point
	known, shown, cursorStyleKnown bool
	cursorStyle                    CursorStyle
	full, left                     bool
}

func (i *Inline) state() inlineState {
	return inlineState{
		i.rows, i.at, i.known, i.shown, i.cursorStyleKnown,
		i.cursorStyle, i.full, i.left,
	}
}

func (i *Inline) restore(s inlineState) {
	i.rows, i.at = s.rows, s.at
	i.known, i.shown, i.cursorStyleKnown = s.known, s.shown, s.cursorStyleKnown
	i.cursorStyle, i.full, i.left = s.cursorStyle, s.full, s.left
}

// Prepare composes immutable permanent contributions and the live frame together.
// Only Commit advances their shared terminal anchor and diff baseline. Refusal
// leaves the caller's publication queue untouched and the old display authoritative.
func (i *Inline) Prepare(contents ...Frozen) (output *Output, err error) {
	i.publication.begin()
	i.buffers.back.endDrawing()
	before := i.state()
	defer func() {
		i.restore(before)
		i.contribution = nil
		if output == nil {
			i.publication.end()
		}
	}()
	if i.publication.failure != nil {
		return nil, i.publication.failure
	}
	width, height := i.buffers.size()
	if err = ValidateTerminalSize(width, height); err != nil {
		return nil, err
	}
	if err = i.buffers.checkTargets(); err != nil {
		return nil, err
	}
	for _, content := range contents {
		rows, rowsErr := content.Rows(width)
		if rowsErr != nil {
			return nil, rowsErr
		}
		i.contribution = append(i.contribution, rows...)
	}
	used := i.used()
	i.buf = i.buf[:0]
	at, err := i.compose(used)
	if err != nil {
		return nil, err
	}
	after := i.state()
	after.rows, after.at, after.full = used, at, false
	var payload []byte
	if len(i.buf) > 0 {
		payload = append(payload, beginSync...)
		payload = append(payload, i.buf...)
		payload = append(payload, endSync...)
		after.left = false
	}
	output = newOutput(payload, func(commit bool) {
		if commit {
			i.restore(after)
			i.buffers.swap()
		}
		i.publication.end()
	})
	return output, nil
}

// Flush writes and commits one synchronous transaction. A failed write poisons
// this canvas; asynchronous runtimes use Prepare and settle after admission.
func (i *Inline) Flush(w io.Writer, contents ...Frozen) error {
	output, err := i.Prepare(contents...)
	if err != nil {
		return err
	}
	defer output.Abort()
	if output.Len() > 0 {
		if err := i.publication.write(w, output.payload); err != nil {
			return err
		}
	}
	output.Commit()
	return nil
}

// PrepareFinish leaves the accepted live block behind. It uses the same writer
// admission transaction as ordinary output, so a refused cleanup does not move
// the remembered anchor.
func (i *Inline) PrepareFinish() (output *Output, err error) {
	i.publication.begin()
	i.buffers.back.endDrawing()
	defer func() {
		if output == nil {
			i.publication.end()
		}
	}()
	if i.publication.failure != nil {
		return nil, i.publication.failure
	}
	if err = ValidateTerminalSize(i.buffers.size()); err != nil {
		return nil, err
	}
	var payload []byte
	if !i.left {
		if i.rows > 0 {
			if i.at.Y > 0 {
				payload = appendCSI(payload, i.at.Y, 'A')
			}
			// Hard line breaks detach accepted rows from the live reflow group
			// before a shell or foreground child moves the cursor elsewhere.
			_, height := i.buffers.size()
			for range min(i.rows, height) {
				payload = append(payload, "\r\n"...)
			}
		}
		payload = append(payload, osc8Close...)
		payload = append(payload, sgrReset...)
		payload = append(payload, defaultCursor...)
		payload = append(payload, showCursor...)
	}
	output = newOutput(payload, func(commit bool) {
		if commit {
			i.rows, i.at = 0, image.Point{}
			i.left = true
			i.invalidate()
		}
		i.publication.end()
	})
	return output, nil
}

// Finish synchronously leaves the accepted block below the terminal cursor.
// It refuses reentry while another output is prepared but unsettled.
func (i *Inline) Finish(w io.Writer) error {
	output, err := i.PrepareFinish()
	if err != nil {
		return err
	}
	defer output.Abort()
	if output.Len() > 0 {
		if err := i.publication.write(w, output.payload); err != nil {
			return err
		}
	}
	output.Commit()
	return nil
}

func (i *Inline) used() int {
	_, h := i.buffers.size()
	last := -1
	for y := h - 1; y >= 0; y-- {
		if len(trimBlankTail(i.buffers.back.row(y))) > 0 {
			last = y
			break
		}
	}
	if i.placed.Visible {
		last = max(last, i.placed.Pos.Y)
	}
	// Native regions contribute height even without cells, otherwise the
	// next relative frame could move above rows the live block still occupies.
	for _, region := range i.buffers.back.regions() {
		last = max(last, region.rect.Max.Y-1)
	}
	return last + 1
}

// compose builds this frame's payload, or leaves it empty when the terminal is
// already showing this frame. It returns where the payload would leave the cursor,
// which is the anchor only a completed write may adopt.
func (i *Inline) compose(used int) (image.Point, error) {
	if !i.compositionNeeded(used) {
		return i.at, nil
	}
	full := i.full || used > i.rows || len(i.contribution) > 0 || i.buffers.regionsNeedRepaint()
	if full {
		if err := eraseRegions(i.buffers.target, &i.buf, i.buffers.front.regions()); err != nil {
			return i.at, err
		}
	}

	// The terminal's style at the start of a frame is not knowable — another program
	// may have written to it — so a frame that writes anything makes it knowable.
	i.buf = append(i.buf, osc8Close...)
	i.buf = append(i.buf, sgrReset...)

	i.buf = append(i.buf, '\r')
	if i.at.Y > 0 {
		i.csi(i.at.Y, 'A')
	}
	advance, err := i.composePending()
	if err != nil {
		return i.at, err
	}
	// A shorter viewport can strand a prefix in scrollback. Visiting more than
	// its height would scroll the replacement block out of reach as well.
	_, height := i.buffers.size()
	extra := max(min(i.rows, height)-advance-used, 0)
	total := i.composeRows(used, extra, full)
	return i.composeEnd(used, total, full)
}

func (i *Inline) compositionNeeded(used int) bool {
	if i.full || i.rows > used || len(i.contribution) > 0 ||
		!sameRegions(i.buffers.front.regions(), i.buffers.back.regions()) || i.cursorPending() {
		return true
	}
	for y := range used {
		if i.rowChanged(y, false) {
			return true
		}
	}
	return false
}

// composePending writes immutable output before the live block. It leaves the
// cursor at the start of the row where the block now begins and reports how far
// that start moved. Only this layout pass decides the rows and margins on the wire.
func (i *Inline) composePending() (int, error) {
	advance := 0
	for _, cells := range i.contribution {
		_, wrapped, err := i.composePrinted(cells, 0)
		if err != nil {
			return 0, err
		}
		i.buf = append(i.buf, "\r\n"...)
		advance += 1 + wrapped
	}
	return advance, nil
}

func (i *Inline) composePrinted(cells []Cell, column int) (int, int, error) {
	width, _ := i.buffers.size()
	wrapped := 0
	for {
		end := 0
		for end < len(cells) {
			unit := paintUnitAt(cells, end, len(cells)-end)
			if unit.width > width-column-end {
				break
			}
			end += unit.width
		}
		if end == 0 && column == 0 && len(cells) > 0 {
			return 0, 0, fmt.Errorf("grid: queued atom width %d exceeds terminal width %d: %w", cells[0].Width(), width, errors.ErrUnsupported)
		}
		i.buf = append(i.buf, encodeCells(cells[:end], i.buffers.depth)...)
		column += end
		if column < width {
			i.buf = append(i.buf, eraseLine...)
		}
		cells = cells[end:]
		if len(cells) == 0 {
			return column, wrapped, nil
		}
		i.buf = append(i.buf, "\r\n"...)
		column = 0
		wrapped++
	}
}

func (i *Inline) composeRows(used, extra int, full bool) int {
	if full {
		i.composeRowGroup(used)
	}
	for y := range used {
		if y > 0 {
			i.csi(1, 'B')
		}
		if !i.rowChanged(y, full) {
			continue
		}
		i.buf = append(i.buf, encodeCells(i.buffers.back.row(y), i.buffers.depth)...)
		i.buf = append(i.buf, '\r')
	}
	for y := used; y < used+extra; y++ {
		if y > 0 {
			i.buf = append(i.buf, "\r\n"...)
		}
		i.buf = append(i.buf, eraseLine...)
	}
	return used + extra
}

// Blank rows establish delayed-autowrap links before painting independent atoms:
// adjacent real rows could otherwise combine at their shared margin. Subsequent
// CR/CUD movement preserves these links; LF or whole-row erasure would sever them.
func (i *Inline) composeRowGroup(rows int) {
	if rows == 0 {
		return
	}
	// A visible suffix left by a height shrink must not stay joined to the
	// old prefix that the terminal has already moved into history.
	i.buf = append(i.buf, eraseLine...)
	width, _ := i.buffers.size()
	for range rows * width {
		i.buf = append(i.buf, ' ')
	}
	i.buf = append(i.buf, '\r')
	if rows > 1 {
		i.csi(rows-1, 'A')
	}
}

func (i *Inline) rowChanged(y int, full bool) bool {
	return full || !rowEqual(i.buffers.front, y, i.buffers.back, y)
}

func (i *Inline) composeEnd(used, total int, full bool) (image.Point, error) {
	// Opaque regions follow restored cells so those cells cannot overwrite
	// them. Cursor restoration follows every painter callback.
	cur, err := i.paintRegions(image.Pt(0, max(total-1, 0)), full)
	if err != nil {
		return i.at, err
	}

	at := image.Pt(0, max(used-1, 0))
	if up := cur.Y - at.Y; up > 0 {
		i.csi(up, 'A')
	}
	if down := at.Y - cur.Y; down > 0 {
		i.csi(down, 'B')
	}
	if cur.X > 0 {
		i.buf = append(i.buf, '\r')
	}
	return i.placeCursor(at), nil
}

// Paint the next ordered composition after its cells, using the same relative
// anchor as every other movement in an inline block.
func (i *Inline) paintRegions(cur image.Point, full bool) (image.Point, error) {
	if !full || len(i.buffers.back.regions()) == 0 {
		return cur, nil
	}
	out := &i.buf
	move := func(to image.Point) {
		if up := cur.Y - to.Y; up > 0 {
			i.csi(up, 'A')
		}
		if down := to.Y - cur.Y; down > 0 {
			i.csi(down, 'B')
		}
		i.buf = append(i.buf, '\r')
		if to.X > 0 {
			i.csi(to.X, 'C')
		}
		cur = to
	}
	if err := paintRegions(i.buffers.target, out, i.buffers.back.regions(), move); err != nil {
		return cur, err
	}
	return cur, nil
}

func (i *Inline) cursorPending() bool {
	if !i.known || i.placed.Visible != i.shown {
		return true
	}
	return i.placed.Visible && (i.placed.Pos != i.at || !i.cursorStyleKnown ||
		i.placed.Style != i.cursorStyle)
}

func (i *Inline) placeCursor(at image.Point) image.Point {
	defer func() { i.known, i.shown = true, i.placed.Visible }()

	if !i.placed.Visible {
		if !i.known || i.shown {
			i.buf = append(i.buf, hideCursor...)
		}
		return at
	}
	style := i.placed.Style
	if !i.cursorStyleKnown || style != i.cursorStyle {
		i.buf = append(i.buf, style.sequence()...)
		i.cursorStyleKnown = true
		i.cursorStyle = style
	}
	// The cursor is never below the block: a frame that placed one counts its row as
	// drawn, which is what makes the block tall enough to hold it.
	if up := at.Y - i.placed.Pos.Y; up > 0 {
		i.csi(up, 'A')
	}
	if i.placed.Pos.X > 0 {
		i.csi(i.placed.Pos.X, 'C')
	}
	if !i.known || !i.shown {
		i.buf = append(i.buf, showCursor...)
	}
	return i.placed.Pos
}

func (i *Inline) csi(n int, final byte) {
	i.buf = appendCSI(i.buf, n, final)
}
