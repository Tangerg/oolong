package grid

import (
	"image"
	"io"
	"strconv"
)

// eraseLine clears from the cursor to the end of its row. It is what makes a row
// that got shorter actually get shorter, rather than keeping the tail of what it
// used to say.
const eraseLine = "\x1b[K"

// Inline draws an interface as a block in the terminal's own screen, with output
// that is finished printed above it.
//
// It is the other way to put frames on a terminal, and the one that makes a
// program part of a session rather than a mode of it: what the interface has
// already said stays in the terminal's own scrollback, where the user can scroll
// back to it, select it, and see it still there after the program exits. A
// [Screen] takes a screen of its own and gives back a blank terminal; this keeps
// the transcript.
//
// # Why nothing here is addressed absolutely
//
// The block's position on the terminal is decided by whatever is above it, which
// this type does not own and cannot ask about. So every frame is written relative
// to where the last one left the cursor: back to the top of the block, down through
// its rows, and back to wherever the cursor belongs. Printing works the same way —
// the rows are written where the block's first row was, and the block is drawn
// below them, which is what pushes finished output up and into the scrollback.
//
// The block is as tall as what was drawn: the rows up to the last one with anything
// on it, and never fewer than enough to hold the cursor. Nothing has to declare a
// height, and an interface that draws two rows occupies two rows.
//
// # What a resize costs
//
// A resize is the one thing this cannot get exactly right. The terminal may reflow
// what is above the block, and there is no way to ask where the block ended up, so
// the next frame repaints in full from where the cursor was left. That is exact
// when the terminal did not reflow and approximate when it did, which is the same
// bargain every inline interface makes.
type Inline struct {
	front, back *Surface
	// scratch is where printed rows are drawn before being encoded. It belongs to
	// the block so it is always the block's width, which is the one width a printed
	// row may be without the terminal wrapping it and moving the block.
	scratch *Surface

	// placed is where this frame's drawing asked the cursor to go, reset by every
	// [Inline.Frame] and read by [Inline.Flush].
	placed Cursor

	// pending is output that has been printed but not yet written, already encoded.
	// A printed row never changes again, so keeping its cells would buy nothing.
	pending []printed

	// open says the last row published has not been finished, and tail is how far
	// along it the output got — the column a continuation of it starts at.
	//
	// Streaming output does not arrive on line boundaries. Without these, every print
	// would begin a row of its own, and a sentence delivered three words at a time
	// would come out three rows tall.
	open bool
	tail int
	// flushed is tail as the terminal has it. It differs from tail while there is
	// output pending, and it is where a continuation of the row the terminal already
	// has open has to start.
	flushed int

	// rows is how tall the block was after the last flush, and at is where that
	// flush left the terminal's cursor, in the block's own coordinates. Together
	// they are the anchor every frame is written relative to.
	rows int
	at   image.Point

	// known and shown are what the terminal has been told about its cursor, so an
	// idle frame says nothing and leaves the blink alone.
	known bool
	shown bool

	// full forces the next flush to rewrite every row of the block, and repaintAll
	// says the same about the regions something else painted — which the rows say
	// nothing about, because what a terminal remembers being shown is not a row.
	full       bool
	repaintAll bool

	// depth is how much colour the terminal is being asked to show.
	depth Depth

	// buf is one frame's payload and out the same wrapped for atomic application.
	buf, out []byte
}

// printed is one piece of published output on its way to the terminal, encoded.
type printed struct {
	row string
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
		front:   NewSurface(w, h),
		back:    NewSurface(w, h),
		scratch: NewSurface(w, 0),
		full:    true,
	}
}

// Size returns the block's width and the height it may grow to.
func (i *Inline) Size() (w, h int) { return i.back.Size() }

// Resize changes the width and the height the block may grow to.
func (i *Inline) Resize(w, h int) {
	if cw, ch := i.back.Size(); cw == w && ch == h {
		return
	}
	i.front.Resize(w, h)
	i.back.Resize(w, h)
	i.Invalidate()
}

// SetDepth says how much colour the terminal can show. It forces a full repaint,
// because every row the terminal is holding was encoded at the old depth.
func (i *Inline) SetDepth(d Depth) {
	if i.depth == d {
		return
	}
	i.depth = d
	i.Invalidate()
}

// SetGround says what the terminal's own two colours are, so that a layer drawn
// over another can mix with it — see [View.Blend].
//
// The scratch surface is told as well as the two that swap: printed output is drawn
// through a view like anything else, and a block of it that dims part of itself
// should dim the same way there as it would in the frame.
func (i *Inline) SetGround(g Ground) {
	i.front.SetGround(g)
	i.back.SetGround(g)
	i.scratch.SetGround(g)
}

// Invalidate forgets what the terminal is showing, so the next flush rewrites the
// whole block.
func (i *Inline) Invalidate() {
	i.full = true
	i.repaintAll = true
	i.known = false
}

// Frame blanks the drawing surface and returns the view for this frame.
//
// The view is as tall as the block may grow to, not as tall as the block ends up:
// its height is decided by what this frame draws into it.
func (i *Inline) Frame() View {
	i.back.Reset()
	i.placed = Cursor{}
	v := i.back.View()
	v.cursor = &i.placed
	return v
}

// Print draws rows that become part of the terminal's own output, above the
// interface, and stay there.
//
// The rows are drawn now, into a surface as wide as the block, and kept as the
// text they came to. They reach the terminal with the next flush, before the block,
// which is what puts them above it.
//
// It takes a count rather than working one out because output can be taller than
// the terminal — a long answer printed into the scrollback is the ordinary case —
// and a caller that has laid its content out already knows how tall it is. Every
// row asked for is printed, so a blank row is a blank row and not slack.
//
// Whole rows, each on a row of its own: a row left open by [Inline.Append] is finished
// first, because what follows it is not part of it. Output arriving in pieces that do
// not stop at a line boundary is that other method's business.
func (i *Inline) Print(rows int, draw func(View)) {
	if rows <= 0 {
		return
	}
	w, _ := i.back.Size()
	i.scratch.Resize(w, rows)
	if draw != nil {
		draw(i.scratch.View())
	}
	for y := range rows {
		i.pending = append(i.pending, printed{row: EncodeRow(i.scratch.row(y), i.depth)})
	}
	i.open, i.tail = false, 0
}

// Append publishes cells onto the end of the row the last one left open, and leaves it
// open for the next.
//
// It is what output that does not arrive on line boundaries needs. A reply streaming in
// three words at a time is one paragraph and not three rows, and a caller with no way
// to say so has to hold everything back until a newline turns up — which for a program
// that never prints one means holding everything back for good.
//
// The view is one row tall and as wide as what is left of the open row, so what is
// drawn into it cannot run past the edge and take the block's anchor with it. A row
// with no room left is finished and the cells go onto the next one: appending means
// putting something after what is there, not squeezing it in beside it.
//
// Drawing nothing publishes nothing, so an empty chunk costs no row.
func (i *Inline) Append(draw func(View)) {
	if draw == nil {
		return
	}
	w, _ := i.back.Size()
	at := 0
	if i.open {
		at = i.tail
	}
	if at >= w {
		at, i.open = 0, false
	}
	i.scratch.Resize(w-at, 1)
	draw(i.scratch.View())
	cells := trimBlankTail(i.scratch.row(0))
	if len(cells) == 0 {
		return
	}
	i.pending = append(i.pending, printed{row: EncodeRow(cells, i.depth), after: i.open})
	i.open, i.tail = true, at+len(cells)
}

// Tail is how far along its row the published output has got, and whether the row is
// open at all. A caller laying text out for [Inline.Append] asks how much of the row is
// already spoken for; one deciding whether it owes a line break asks the second answer.
func (i *Inline) Tail() (col int, open bool) { return i.tail, i.open }

// Break finishes the open row, so that what is published next begins one of its own.
//
// It writes nothing. The row was published with the block underneath it already, so
// ending it is only a matter of not carrying it on — which is why a caller can break a
// row it has changed its mind about at no cost.
func (i *Inline) Break() { i.open, i.tail = false, 0 }

// Cursor is where the last frame asked for the terminal's cursor to go, in the
// block's own coordinates. See [Screen.Cursor].
func (i *Inline) Cursor() Cursor { return i.placed }

// Flush writes this frame to w, leaving the cursor wherever the frame placed it.
//
// A flush that would change nothing writes nothing at all, for the same reason a
// [Screen] does: an idle interface should be silent on the wire and should leave
// the cursor's blink undisturbed.
func (i *Inline) Flush(w io.Writer) error {
	used := i.used()
	i.buf = i.buf[:0]
	if err := i.compose(used); err != nil {
		// The frame never reached w, but a painter may have emitted a partial private
		// payload before failing. Forget all terminal assumptions and preserve pending
		// output so the caller may either report the failure or deliberately retry.
		i.Invalidate()
		return err
	}
	if len(i.buf) == 0 {
		i.settle(used)
		return nil
	}
	i.out = append(i.out[:0], beginSync...)
	i.out = append(i.out, i.buf...)
	i.out = append(i.out, endSync...)
	if err := writeAll(w, i.out); err != nil {
		// Some prefix of the frame may have landed, so what the terminal is showing
		// is no longer known. The printed rows are kept rather than dropped: output
		// the caller asked for is worth writing twice and not worth losing.
		i.Invalidate()
		return err
	}
	clear(i.pending)
	i.pending = i.pending[:0]
	// What the terminal now has open is what was pending a moment ago.
	i.flushed = i.tail
	i.settle(used)
	return nil
}

// Finish leaves the block on screen with the cursor below it, so whatever writes
// next — the shell's prompt, or this program's own output — starts on a line of its
// own instead of on top of the interface.
//
// It is the counterpart of giving back the alternate screen, and the reason an
// inline program has to draw one last frame before it exits: the last thing it
// showed is the thing that stays.
func (i *Inline) Finish(w io.Writer) error {
	i.buf = i.buf[:0]
	if i.rows > 0 {
		if down := i.rows - 1 - i.at.Y; down > 0 {
			i.csi(down, 'B')
		}
		i.buf = append(i.buf, "\r\n"...)
	}
	i.buf = append(i.buf, sgrReset...)
	i.buf = append(i.buf, showCursor...)
	i.rows, i.at = 0, image.Point{}
	// Whatever was left open stays as it is — it is the terminal's output now — but
	// nothing further belongs on it: the cursor has moved below the block.
	i.open, i.tail, i.flushed = false, 0, 0
	i.Invalidate()
	return writeAll(w, i.buf)
}

// used is how many rows the block needs: the rows up to the last one with anything
// on it, and enough to hold the cursor if drawing placed one.
func (i *Inline) used() int {
	_, h := i.back.Size()
	last := -1
	for y := h - 1; y >= 0; y-- {
		if len(trimBlankTail(i.back.row(y))) > 0 {
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
	for _, region := range i.back.regions() {
		last = max(last, region.rect.Max.Y-1)
	}
	return last + 1
}

// compose builds this frame's payload, or leaves it empty when the terminal is
// already showing this frame.
func (i *Inline) compose(used int) error {
	// Printing rewrites the rows the block's first rows were on and moves the block
	// down past them, so nothing the block is showing survives it. A piece that goes
	// onto the end of a row already published moves nothing: the row it goes on is
	// above the block already.
	advance := 0
	for _, p := range i.pending {
		if !p.after {
			advance++
		}
	}
	full := i.full || advance > 0

	changed := func(y int) bool { return full || !rowEqual(i.front, y, i.back, y) }

	// The rows the block is giving up. They are erased where they are rather than
	// deleted, which leaves the block shorter with blank rows below it and moves
	// nothing that is above it.
	extra := max(i.rows-advance-used, 0)

	work := full || extra > 0 || len(i.pending) > 0 ||
		!sameRegions(i.front.regions(), i.back.regions())
	for y := 0; y < used && !work; y++ {
		work = changed(y)
	}
	if !work && !i.cursorPending() {
		return nil
	}

	// The terminal's style at the start of a frame is not knowable — another program
	// may have written to it — so a frame that writes anything makes it knowable.
	i.buf = append(i.buf, sgrReset...)

	// Back to the top of the block, which is the only position this type can name.
	i.buf = append(i.buf, '\r')
	if i.at.Y > 0 {
		i.csi(i.at.Y, 'A')
	}

	for k, p := range i.pending {
		switch {
		case k == 0 && p.after:
			// The row being carried on is the one above the block: it was published
			// before the block was drawn underneath it.
			i.csi(1, 'A')
			if i.flushed > 0 {
				i.csi(i.flushed, 'C')
			}
		case k > 0 && !p.after:
			i.buf = append(i.buf, "\r\n"...)
		}
		i.buf = append(i.buf, p.row...)
		i.buf = append(i.buf, eraseLine...)
	}
	if len(i.pending) > 0 {
		// Below everything published, which is where the block goes.
		i.buf = append(i.buf, "\r\n"...)
	}

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
		case changed(y):
			row := EncodeRow(i.back.row(y), i.depth)
			i.buf = append(i.buf, row...)
			i.buf = append(i.buf, eraseLine...)
			moved = len(row) > 0
		}
	}
	if moved {
		i.buf = append(i.buf, '\r')
	}

	// The regions something else paints, from where the rows left the cursor. They
	// go after the rows for the reason they do on a screen — the rows would write the
	// blanks they think are underneath over what was painted — and before the cursor,
	// which has to end up where this frame asked whatever a painter did on the way.
	cur, err := i.paintRegions(image.Pt(0, max(total-1, 0)), full)
	if err != nil {
		return err
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
	i.placeCursor(at)
	return nil
}

// paintRegions writes what turns the regions the terminal is showing into the ones
// this frame asked for, and reports where it left the cursor.
//
// Everything is relative, like every other movement in an inline block. A frame that
// published output moved the block down past it, which moved what the terminal is
// showing with it, so that frame paints its regions again from nothing — the same
// answer a full repaint gives, and for the same reason.
func (i *Inline) paintRegions(cur image.Point, full bool) (image.Point, error) {
	was, now := i.front.regions(), i.back.regions()
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
	return i.placed.Visible && i.placed.Pos != i.at
}

// placeCursor moves the cursor from at, where writing the rows left it, to where
// this frame's drawing asked for it.
func (i *Inline) placeCursor(at image.Point) {
	i.at = at
	defer func() { i.known, i.shown = true, i.placed.Visible }()

	if !i.placed.Visible {
		if !i.known || i.shown {
			i.buf = append(i.buf, hideCursor...)
		}
		return
	}
	// The cursor is never below the block: a frame that placed one counts its row as
	// drawn, which is what makes the block tall enough to hold it.
	if up := at.Y - i.placed.Pos.Y; up > 0 {
		i.csi(up, 'A')
	}
	if i.placed.Pos.X > 0 {
		i.csi(i.placed.Pos.X, 'C')
	}
	i.at = i.placed.Pos
	if !i.known || !i.shown {
		i.buf = append(i.buf, showCursor...)
	}
}

// settle makes this frame the one the terminal is showing.
func (i *Inline) settle(used int) {
	i.front, i.back = i.back, i.front
	i.rows = used
	i.full = false
}

func (i *Inline) csi(n int, final byte) {
	i.buf = append(i.buf, '\x1b', '[')
	i.buf = strconv.AppendInt(i.buf, int64(n), 10)
	i.buf = append(i.buf, final)
}
