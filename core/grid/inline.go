package grid

import (
	"errors"
	"fmt"
	"image"
	"io"
	"slices"
)

const eraseLine = "\x1b[K"

var errScrollbackPaint = fmt.Errorf("grid: scrollback cannot retain native paint regions: %w", errors.ErrUnsupported)

// Inline keeps finished output in terminal scrollback and draws the active block
// relative to its last cursor anchor. Terminal reflow on resize requires a full
// repaint; absolute placement is unavailable. An Inline belongs to one goroutine
// and must not be copied after first use. Print, Append, Flush and Finish callbacks
// may read the canvas and draw their supplied view, but cannot mutate retained frame
// views or reenter mutating Inline methods; violations panic.
type Inline struct {
	noCopy      noCopy
	publication publication

	buffers frameBuffer
	scratch *Surface

	placed Cursor

	// Accepted cells remain immutable until Flush lays them out at the current width.
	pending []printed

	// Zero closes the append row; a positive column is its retained open tail.
	tail int
	// Pending output can advance tail before the terminal receives it; continuations
	// must start at the last published column.
	flushed int

	// Only successful writes advance the terminal anchor. A failed write makes relative
	// placement unknowable and permanently ends the session.
	rows int
	at   image.Point

	// Unchanged visibility writes nothing, preserving cursor blink.
	known bool
	shown bool
	// Visibility and shape are independent; hiding does not reset shape.
	cursorStyleKnown bool
	cursorStyle      CursorStyle

	// Cell rows do not describe terminal-native paint, so a full repaint must invalidate both.
	full       bool
	repaintAll bool

	// left says the block has been finished with and nothing has been written since,
	// so finishing it again has nothing to do. Whether the terminal has been given the
	// block back is this type's fact: a caller that has to remember whether it already
	// asked would be keeping a second copy of it, and two callers with reason to ask —
	// an exit and a handover — would each keep their own.
	left bool

	// buf is one frame's payload and out the same wrapped for atomic application.
	buf, out []byte
}

// printed is one piece of accepted output on its way to the terminal.
type printed struct {
	cells []Cell
	// after says this goes onto the end of what was published before it rather than
	// onto a row of its own. It is what a block that does not begin at a column is:
	// the cells go where the last ones stopped.
	after bool
}

// NewInline returns an inline block that may grow to h rows of w columns, whose
// first flush draws everything.
//
// The height is a ceiling rather than a size: it is what the terminal can spare,
// and the block takes as much of it as the interface draws into.
func NewInline(w, h int) *Inline {
	return &Inline{
		buffers: newFrameBuffer(w, h),
		scratch: NewSurface(w, 0),
		full:    true,
	}
}

// Size returns the block's width and the height it may grow to.
func (i *Inline) Size() (w, h int) { return i.buffers.size() }

// Resize changes the width and the height the block may grow to. A width change
// ends the open append row: terminal reflow cannot preserve its old column anchor.
// Accepted cells remain queued and wrap at the new width when flushed.
func (i *Inline) Resize(w, h int) {
	i.publication.requireIdle()
	oldWidth, _ := i.buffers.size()
	if i.buffers.resize(w, h) {
		if width, _ := i.buffers.size(); width != oldWidth {
			if len(i.pending) > 0 {
				i.pending[0].after = false
			}
			i.tail, i.flushed = 0, 0
		}
		i.Invalidate()
	}
}

// SetDepth says how much colour the terminal can show. It forces a full repaint,
// because every row the terminal is holding was encoded at the old depth.
func (i *Inline) SetDepth(d Depth) {
	i.publication.requireIdle()
	if i.buffers.setDepth(d) {
		i.Invalidate()
	}
}

// SetGround says what the terminal's own two colours are, so that a layer drawn
// over another can mix with it — see [View.Blend].
//
// The scratch surface is told as well as the two that swap: printed output is drawn
// through a view like anything else, and a block of it that dims part of itself
// should dim the same way there as it would in the frame.
func (i *Inline) SetGround(g Ground) {
	i.publication.requireIdle()
	i.buffers.setGround(g)
	i.scratch.SetGround(g)
}

// Invalidate forgets what the terminal is showing, so the next flush rewrites the
// whole block.
func (i *Inline) Invalidate() {
	i.publication.requireIdle()
	i.invalidate()
}

func (i *Inline) invalidate() {
	i.full = true
	i.repaintAll = true
	i.known = false
	i.cursorStyleKnown = false
}

// Frame blanks the drawing surface and returns the view for this frame.
//
// The view is as tall as the block may grow to, not as tall as the block ends up:
// its height is decided by what this frame draws into it.
// The view and its subviews may be written until the next Frame, a size change,
// or Flush or Finish begins. Print and Append temporarily exclude frame drawing.
// Writes outside that lifetime panic.
func (i *Inline) Frame() View {
	i.publication.requireIdle()
	return i.buffers.begin(&i.placed, &i.publication)
}

// Print queues complete rows above the interface for the next flush. It finishes
// any row left open by Append so unrelated output cannot join it. Nil means the
// content was accepted, including empty content; delivery belongs to Flush. After
// a destination write fails, Print returns that failure without calling content.
// Native paint regions require a live frame; they refuse the entire publication
// with errors.ErrUnsupported, leaving pending output and the open row unchanged.
// The drawing view and its subviews expire when Draw returns, including abnormally.
func (i *Inline) Print(content Drawable) error {
	i.publication.begin()
	defer i.publication.end()
	if i.publication.failure != nil {
		return i.publication.failure
	}
	if content == nil {
		return nil
	}
	w, _ := i.buffers.size()
	rows := content.HeightForWidth(w)
	if rows <= 0 {
		return nil
	}
	i.scratch.Resize(w, rows)
	defer i.scratch.Reset()
	content.Draw(i.scratch.borrow(nil))
	if len(i.scratch.regions()) > 0 {
		return errScrollbackPaint
	}
	for y := range rows {
		cells := slices.Clone(trimBlankTail(i.scratch.row(y)))
		i.pending = append(i.pending, printed{cells: cells})
	}
	i.tail = 0
	return nil
}

// Append extends an unfinished row until draw returns false. Its view contains the
// remaining columns; true continues on a new row. No rows are accepted until all
// callbacks return normally. Empty drawing with false is a no-op; requesting another
// row without drawing in a whole empty row returns io.ErrNoProgress.
// Nil reports acceptance, not delivery. A destination failure refuses without drawing.
// Native paint regions refuse the entire append with errors.ErrUnsupported.
// Each drawing view and its subviews expire when that callback returns.
func (i *Inline) Append(draw func(View) bool) error {
	i.publication.begin()
	defer i.publication.end()
	if i.publication.failure != nil {
		return i.publication.failure
	}
	if draw == nil {
		return nil
	}
	w, _ := i.buffers.size()
	defer i.scratch.Reset()
	tail := i.tail
	var pending []printed
	for {
		at, after := tail, tail > 0
		if at >= w {
			at, after = 0, false
		}
		i.scratch.Resize(w-at, 1)
		more := draw(i.scratch.borrow(nil))
		if len(i.scratch.regions()) > 0 {
			return errScrollbackPaint
		}
		cells := trimBlankTail(i.scratch.row(0))
		if len(cells) > 0 {
			pending = append(pending, printed{
				cells: slices.Clone(cells), after: after,
			})
			tail = at + len(cells)
		}
		if !more {
			break
		}
		if len(cells) == 0 && at == 0 {
			return io.ErrNoProgress
		}
		tail = 0
	}
	i.pending = append(i.pending, pending...)
	i.tail = tail
	return nil
}

// Tail is how far along its row the published output has got, and whether the row is
// open at all. A caller laying text out for [Inline.Append] asks how much of the row is
// already spoken for; one deciding whether it owes a line break asks the second answer.
func (i *Inline) Tail() (col int, open bool) { return i.tail, i.tail > 0 }

// Break finishes the open row, so that what is published next begins one of its own.
//
// It writes nothing. The row was published with the block underneath it already, so
// ending it is only a matter of not carrying it on — which is why a caller can break a
// row it has changed its mind about at no cost.
func (i *Inline) Break() {
	i.publication.requireIdle()
	i.tail = 0
}

// Cursor is where the last frame asked for the terminal's cursor to go, in the
// block's own coordinates. See [Screen.Cursor].
func (i *Inline) Cursor() Cursor { return i.placed }

// Flush writes changed frames and places the cursor; unchanged frames write nothing.
// A zero width defers output until a positive width is available.
// A queued atom wider than the whole terminal returns errors.ErrUnsupported before
// writing, retaining accepted output for a retry at a width that can represent it.
// Failed writes permanently end the session because partial output loses the relative
// anchor and placement inventory. Flush and Finish then return the original error
// without writing. Abnormal writer exits also end publication.
func (i *Inline) Flush(w io.Writer) error {
	i.publication.begin()
	defer i.publication.end()
	i.buffers.back.endDrawing()
	if i.publication.failure != nil {
		return i.publication.failure
	}
	if width, _ := i.buffers.size(); width == 0 {
		return nil
	}
	composed := false
	defer func() {
		if !composed {
			i.invalidate()
		}
	}()
	used := i.used()
	i.buf = i.buf[:0]
	at, err := i.compose(used)
	if err != nil {
		// The frame never reached w, but a painter may have emitted a partial private
		// payload before failing. Forget all terminal assumptions and preserve pending
		// output so the caller may either report the failure or deliberately retry.
		return err
	}
	composed = true
	if len(i.buf) == 0 {
		i.settle(used, at)
		return nil
	}
	i.out = append(i.out[:0], beginSync...)
	i.out = append(i.out, i.buf...)
	i.out = append(i.out, endSync...)
	if err := i.publication.write(w, i.out); err != nil {
		return err
	}
	clear(i.pending)
	i.pending = i.pending[:0]
	i.left = false
	// What the terminal now has open is what was pending a moment ago.
	i.flushed = i.tail
	i.settle(used, at)
	return nil
}

// Finish leaves the block on screen with the cursor below it, so whatever writes
// next — the shell's prompt, or this program's own output — starts on a line of its
// own instead of on top of the interface.
//
// It is the counterpart of giving back the alternate screen, and the reason an
// inline program has to draw one last frame before it exits: the last thing it
// showed is the thing that stays.
// Unflushed output refuses Finish before any write or anchor change. Flush it
// successfully before retrying; the accepted cells and open row remain intact.
func (i *Inline) Finish(w io.Writer) error {
	i.publication.begin()
	defer i.publication.end()
	i.buffers.back.endDrawing()
	if i.publication.failure != nil {
		return i.publication.failure
	}
	if len(i.pending) > 0 {
		return errors.New("grid: cannot finish with unflushed output")
	}
	if i.left {
		return nil
	}
	i.buf = i.buf[:0]
	if i.rows > 0 {
		if down := i.rows - 1 - i.at.Y; down > 0 {
			i.csi(down, 'B')
		}
		i.buf = append(i.buf, "\r\n"...)
	}
	i.buf = append(i.buf, osc8Close...)
	i.buf = append(i.buf, sgrReset...)
	i.buf = append(i.buf, defaultCursor...)
	i.buf = append(i.buf, showCursor...)
	if err := i.publication.write(w, i.buf); err != nil {
		return err
	}
	i.rows, i.at = 0, image.Point{}
	// Whatever was left open stays as it is — it is the terminal's output now — but
	// nothing further belongs on it: the cursor has moved below the block.
	i.tail, i.flushed = 0, 0
	i.left = true
	i.invalidate()
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
	// A region something else paints has no cells in it, and the block still has to
	// be tall enough to hold it: a picture below the block's last written row would
	// be drawn outside it, and the next frame would move up over rows it does not
	// own.
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
	full := i.full || advance > 0
	// Clear released live rows after accounting for the physical rows that accepted
	// output now occupies, including wrapping caused by a width change.
	extra := max(i.rows-advance-used, 0)
	total := i.composeRows(used, extra, full)
	return i.composeEnd(used, total, full)
}

func (i *Inline) settle(used int, at image.Point) {
	i.buffers.swap()
	i.rows = used
	i.at = at
	i.full = false
}

func (i *Inline) compositionNeeded(used int) bool {
	if i.full || i.rows > used || len(i.pending) > 0 ||
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
	column, advance := 0, 0
	for k, p := range i.pending {
		switch {
		case k == 0 && p.after:
			// The row being carried on is the one above the block: it was published
			// before the block was drawn underneath it.
			i.csi(1, 'A')
			if i.flushed > 0 {
				i.csi(i.flushed, 'C')
			}
			column = i.flushed
		case k > 0 && !p.after:
			i.buf = append(i.buf, "\r\n"...)
		}
		if !p.after {
			column = 0
			advance++
		}
		nextColumn, wrapped, err := i.composePrinted(p.cells, column)
		if err != nil {
			return 0, err
		}
		column = nextColumn
		advance += wrapped
	}
	if len(i.pending) > 0 {
		i.buf = append(i.buf, "\r\n"...)
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

// composeRows rewrites changed live rows and erases rows the block released. It
// returns how many rows were visited, with the cursor at column zero on the last.
func (i *Inline) composeRows(used, extra int, full bool) int {
	// moved tracks whether the row just visited left the cursor away from column
	// zero, which is the only thing a carriage return is for.
	moved := false
	total := used + extra
	for y := range total {
		if y > 0 {
			// The carriage return cancels the terminal's pending wrap before the
			// newline: a row that filled the last column would otherwise advance
			// twice and take the block's anchor with it.
			i.buf = append(i.buf, "\r\n"...)
			moved = false
		}
		switch {
		case y >= used:
			// Erasing leaves the cursor where it is, which is already column zero.
			i.buf = append(i.buf, eraseLine...)
		case i.rowChanged(y, full):
			row, atMargin := encodeInlineRow(i.buffers.back.row(y), i.buffers.depth)
			i.buf = append(i.buf, row...)
			if !atMargin {
				i.buf = append(i.buf, eraseLine...)
			}
			moved = len(row) > 0
		}
	}
	if moved {
		i.buf = append(i.buf, '\r')
	}
	return total
}

// encodeInlineRow reports whether the encoded cells reach the terminal's right
// margin. A terminal keeps its cursor on the last cell while wrap is pending, so
// erase-to-end there would erase the last character rather than clear an empty tail.
func encodeInlineRow(cells []Cell, depth Depth) (row string, atMargin bool) {
	visible := trimBlankTail(cells)
	return EncodeRow(visible, depth), len(visible) > 0 && len(visible) == len(cells)
}

func (i *Inline) rowChanged(y int, full bool) bool {
	return full || !rowEqual(i.buffers.front, y, i.buffers.back, y)
}

func (i *Inline) composeEnd(used, total int, full bool) (image.Point, error) {
	// The regions something else paints, from where the rows left the cursor. They
	// go after the rows for the reason they do on a screen — the rows would write the
	// blanks they think are underneath over what was painted — and before the cursor,
	// which has to end up where this frame asked whatever a painter did on the way.
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

// paintRegions writes what turns the regions the terminal is showing into the ones
// this frame asked for, and reports where it left the cursor.
//
// Everything is relative, like every other movement in an inline block. A frame that
// published output moved the block down past it, which moved what the terminal is
// showing with it, so that frame paints its regions again from nothing — the same
// answer a full repaint gives, and for the same reason.
func (i *Inline) paintRegions(cur image.Point, full bool) (image.Point, error) {
	was, now := i.buffers.front.regions(), i.buffers.back.regions()
	if len(was) == 0 && len(now) == 0 {
		return cur, nil
	}
	out := bytesTo{&i.buf}
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
	if full || i.repaintAll {
		i.repaintAll = false
		if err := repaint(out, was, nil, move); err != nil {
			return cur, err
		}
		was = nil
	}
	if err := repaint(out, was, now, move); err != nil {
		return cur, err
	}
	return cur, nil
}

// cursorPending reports whether the terminal has to be told something about its
// cursor that it does not already know.
func (i *Inline) cursorPending() bool {
	if !i.known || i.placed.Visible != i.shown {
		return true
	}
	return i.placed.Visible && (i.placed.Pos != i.at || !i.cursorStyleKnown ||
		i.placed.Style.normalized() != i.cursorStyle)
}

func (i *Inline) placeCursor(at image.Point) image.Point {
	defer func() { i.known, i.shown = true, i.placed.Visible }()

	if !i.placed.Visible {
		if !i.known || i.shown {
			i.buf = append(i.buf, hideCursor...)
		}
		return at
	}
	style := i.placed.Style.normalized()
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
