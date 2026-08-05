package headless

import (
	"image"

	"github.com/Tangerg/oolong/core/grid"
)

// Transcript is a growing record of output, in one coordinate space.
//
// It is what everything that has to talk about a position in a session's output talks
// about. A scroll offset, the ends of a selection, a search match, a prompt pinned to
// the top of the view — all of them are rows, and they only mean the same thing to
// each other if there is one numbering they all use. That is what this holds: an
// ordered list of blocks, each of a height that depends on the width, and the row
// each of them starts at.
//
// # Why the output has to be held at all
//
// An inline interface can print output and let the terminal keep it, which is the
// right answer for output nobody will touch again. Text the terminal owns cannot be
// selected by the program, searched, re-wrapped when the window changes, or scrolled
// back over under the program's own control — the terminal does all of that, in its
// own way, and tells the program nothing. A transcript is for output the program
// means to keep answering questions about.
//
// # What it costs
//
// Appending is constant time. So is a block growing at the end, which is what a
// streaming answer does token by token: only that block is measured again, and only
// the rows after it move. A change of width is the one linear operation, because a
// width is what every height is a function of.
//
// The zero value is an empty transcript at width zero. Nothing can be measured until
// [Transcript.Resize] has said how wide the space is.
type Transcript struct {
	blocks []placed
	width  int
	total  int
}

// placed is a block, its height at the transcript's width, and where it sits.
//
// The top is stored rather than summed on demand. Every question asked of a
// transcript — which block owns this row, which rows does this block cover, what is
// visible — is a question about tops, and a session with ten thousand blocks would
// otherwise add ten thousand numbers to answer each one.
type placed struct {
	block  Sized
	height int
	top    int
}

// Len is how many blocks the transcript holds.
func (t *Transcript) Len() int { return len(t.blocks) }

// Rows is the total height of everything in it, at the current width.
func (t *Transcript) Rows() int { return t.total }

// Width is the width every height in it was measured at.
func (t *Transcript) Width() int { return t.width }

// Append adds a block at the end and measures it.
func (t *Transcript) Append(b Sized) {
	if b == nil {
		return
	}
	height := t.measure(b)
	t.blocks = append(t.blocks, placed{block: b, height: height, top: t.total})
	t.total += height
}

// Block is the block at index i, or nil when there is none there.
func (t *Transcript) Block(i int) Sized {
	if i < 0 || i >= len(t.blocks) {
		return nil
	}
	return t.blocks[i].block
}

// Last is the block most recently appended, or nil when the transcript is empty. It
// is the one a streaming answer is still arriving into.
func (t *Transcript) Last() Sized {
	if len(t.blocks) == 0 {
		return nil
	}
	return t.blocks[len(t.blocks)-1].block
}

// Changed says the block at i has new content, and re-measures from there.
//
// It has to be said rather than noticed. A block is an ordinary mutable object and
// nothing here is told when its text changes, so the alternative is measuring every
// block on every frame — which is the one thing this structure exists to avoid.
//
// Everything before i keeps the height it had, because nothing before it moved.
func (t *Transcript) Changed(i int) {
	if i < 0 || i >= len(t.blocks) {
		return
	}
	t.remeasure(i)
}

// Resize measures everything again at a new width, and reports the new total.
//
// A width is what every height in here is a function of, so this is linear in the
// number of blocks and there is no version of it that is not. Calling it with the
// width it already has costs nothing, which is what makes it safe to call once a
// frame — and what a caller should do, rather than trying to remember whether the
// window changed.
func (t *Transcript) Resize(width int) int {
	if width == t.width {
		return t.total
	}
	t.width = width
	t.remeasure(0)
	return t.total
}

// remeasure recomputes heights and tops from i onwards.
func (t *Transcript) remeasure(from int) {
	top := 0
	if from > 0 {
		prev := t.blocks[from-1]
		top = prev.top + prev.height
	}
	for i := from; i < len(t.blocks); i++ {
		t.blocks[i].height = t.measure(t.blocks[i].block)
		t.blocks[i].top = top
		top += t.blocks[i].height
	}
	t.total = top
}

// measure is a block's height at the current width, never negative.
func (t *Transcript) measure(b Sized) int {
	if t.width <= 0 {
		return 0
	}
	return max(b.Measure(t.width), 0)
}

// Extent is the rows block i covers: the first, and how many.
//
// It reports false for an index that is not there, which is what a caller looping
// over indices it kept from an earlier frame needs — a transcript only grows, but a
// caller holding an index into one it does not own has no way to know that.
func (t *Transcript) Extent(i int) (top, height int, ok bool) {
	if i < 0 || i >= len(t.blocks) {
		return 0, 0, false
	}
	return t.blocks[i].top, t.blocks[i].height, true
}

// At is the block covering a row, and how far into that block the row is.
//
// The search is a bisection over the tops rather than a walk, because this is asked
// once per click, once per selection end, and once per frame for the top of the
// view — and a session's transcript grows without bound while a screen does not.
func (t *Transcript) At(row int) (i, offset int, ok bool) {
	if row < 0 || row >= t.total {
		return 0, 0, false
	}
	i = t.index(row)
	return i, row - t.blocks[i].top, true
}

// index is the block covering a row, which must be a row that exists.
//
// It is shared with [Transcript.Visible] rather than repeated there. Two bisections
// over the same tops would eventually disagree about a run of blocks with no height,
// and the transcript that exercised the difference is the one nobody built.
func (t *Transcript) index(row int) int {
	// The last block whose top is at or below the row.
	lo, hi := 0, len(t.blocks)-1
	for lo < hi {
		mid := (lo + hi + 1) / 2
		if t.blocks[mid].top <= row {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	// The answer is never a block of no height, and nothing here has to check.
	//
	// Such a block shares its top with whichever block follows it, and this finds the
	// largest index at or below the row — so the one it lands on is the last of any
	// run sharing that top, which is the one with height. A run at the very end has
	// nowhere to share with, and its top is the total, which the caller has already
	// ruled the row out of.
	return lo
}

// Visible is the range of blocks that any of the rows [from, from+rows) touch.
//
// The end is exclusive. An empty range comes back as two equal indices, which is what
// a viewport scrolled past everything gives.
func (t *Transcript) Visible(from, rows int) (first, last int) {
	if rows <= 0 || t.total == 0 || from >= t.total {
		return len(t.blocks), len(t.blocks)
	}
	from = max(from, 0)
	first = t.index(from)
	end := from + rows
	last = first
	for last < len(t.blocks) && t.blocks[last].top < end {
		last++
	}
	return first, last
}

// Draw writes the window of rows starting at from into v, which is as many rows tall
// as the window is.
//
// A block that the window cuts into is drawn whole into a view that starts above or
// ends below the space available, and the parts outside are discarded — which is what
// a view already does, and is why nothing here has to teach a block about being
// partly visible. A block does not know it is in a transcript at all.
func (t *Transcript) Draw(v grid.View, from int) {
	w, h := v.Size()
	if w <= 0 || h <= 0 {
		return
	}
	first, last := t.Visible(from, h)
	for i := first; i < last; i++ {
		b := t.blocks[i]
		if b.height == 0 {
			continue
		}
		y := b.top - from
		b.block.Draw(v.Sub(image.Rect(0, y, w, y+b.height)))
	}
}

// Copyable is a block that can say what it draws, so a selection can be copied out
// of it.
//
// It is separate from drawing because copying is not drawing: what a user expects on
// the clipboard is the text, without the box around it, without the accent in the
// gutter, and without the padding that made it look right. A block that cannot answer
// this contributes blank rows to a selection rather than nothing at all — it still
// occupies the rows, and a selection dragged across it has to come out with the same
// number of lines the user dragged over.
type Copyable interface {
	// Text is what the block's rows say at a width, one string per row, and as many
	// of them as [layout.Measurer.Measure] reports at that width.
	Text(width int) []string
}

// Text is what the transcript's rows say, one string per row, for the rows
// [from, from+rows).
//
// Rows belonging to a block that cannot be copied come back empty. The count is
// always the number of rows asked for, clamped to what exists, so a caller can index
// the result by row and get the row it meant.
func (t *Transcript) Text(from, rows int) []string {
	if rows <= 0 || from >= t.total {
		return nil
	}
	from = max(from, 0)
	rows = min(rows, t.total-from)
	out := make([]string, rows)

	first, last := t.Visible(from, rows)
	for i := first; i < last; i++ {
		b := t.blocks[i]
		if b.height == 0 {
			continue
		}
		copyable, ok := b.block.(Copyable)
		if !ok {
			continue
		}
		lines := copyable.Text(t.width)
		for row := range b.height {
			at := b.top + row - from
			if at < 0 || at >= rows || row >= len(lines) {
				continue
			}
			out[at] = lines[row]
		}
	}
	return out
}
