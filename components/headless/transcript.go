package headless

import (
	"image"

	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/layout"
	"github.com/Tangerg/oolong/core/text"
)

// Transcript is the live, retained part of output, in one coordinate space.
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
	rows   int
	start  int
	first  BlockID

	pendingLayout transcriptLayoutState
	staged        *transaction
}

// BlockID is the stable identity of one block in a [Transcript].
//
// IDs increase as blocks are appended and are never reused. Committing a leading
// block invalidates its ID without changing the IDs of blocks that remain live. That
// is what lets a sticky header or another retained part keep referring to a live block
// while committed storage is physically removed from the transcript.
type BlockID int

// placed is a block, its height at the transcript's width, and where it sits.
//
// The top is stored rather than summed on demand. Every question asked of a
// transcript — which block owns this row, which rows does this block cover, what is
// visible — is a question about tops, and a session with ten thousand blocks would
// otherwise add ten thousand numbers to answer each one.
type placed struct {
	block  Block
	height int
	top    int
	// finished says the block will not change again, which is what makes it eligible
	// to be given to the terminal. See [Transcript.Commit].
	finished bool
}

// Len is how many live blocks the transcript holds.
func (t *Transcript) Len() int { return len(t.blocks) }

// Height is the height of the live blocks at the current width.
func (t *Transcript) Height() int { return t.rows }

// StartRow is the first row the transcript still owns. Earlier rows have been
// committed to the terminal and cannot be drawn, searched, selected, or rewrapped.
func (t *Transcript) StartRow() int { return t.start }

// EndRow is the exclusive end of the transcript's live row range.
func (t *Transcript) EndRow() int { return layout.Sum(t.start, t.rows) }

// FirstBlock is the ID of the first live block. When the transcript is empty it is
// the ID the next appended block will receive.
func (t *Transcript) FirstBlock() BlockID { return t.first }

// Width is the width every height in it was measured at.
func (t *Transcript) Width() int { return t.width }

// Append adds a block at the end, measures it, and returns its stable identity.
// Appending nil changes nothing and returns the next available identity.
func (t *Transcript) Append(b Block) BlockID {
	id := t.first + BlockID(len(t.blocks))
	if b == nil {
		return id
	}
	height := t.measure(b)
	t.blocks = append(t.blocks, placed{block: b, height: height, top: t.EndRow()})
	t.rows = layout.Sum(t.rows, height)
	return id
}

// Block is the live block with id, or nil when there is none.
func (t *Transcript) Block(id BlockID) Block {
	i, ok := t.position(id)
	if !ok {
		return nil
	}
	return t.blocks[i].block
}

// Last is the block most recently appended, or nil when the transcript is empty. It
// is the one a streaming answer is still arriving into.
func (t *Transcript) Last() Block {
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
func (t *Transcript) Changed(id BlockID) {
	i, ok := t.position(id)
	if !ok {
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
		return t.rows
	}
	t.width = width
	t.remeasure(0)
	return t.rows
}

// Layout returns the transcript's currently committed geometry.
func (t *Transcript) Layout() TranscriptLayout {
	if t == nil {
		return TranscriptLayout{}
	}
	return TranscriptLayout{state: transcriptLayoutState{
		blocks: t.blocks,
		width:  t.width,
		rows:   t.rows,
		start:  t.start,
		first:  t.first,
	}}
}

// Stage lays the transcript out at width for a component frame.
//
// A changed width is measured into private pending placement. The new row space
// becomes observable through Layout, Height, Extent, selection and search only when
// the complete Root frame commits. Calling Stage at the committed width reuses the
// existing placement without allocation.
func (t *Transcript) Stage(frame Frame, width int) TranscriptLayout {
	if t == nil {
		return TranscriptLayout{}
	}
	width = max(width, 0)
	if t.staged == nil && width == t.width {
		return t.Layout()
	}
	if frame.transaction == nil || !frame.transaction.active {
		panic("headless: transcript layout staged outside Root.Draw")
	}
	if t.staged != frame.transaction {
		if t.staged != nil {
			panic("headless: transcript layout staged by two roots")
		}
		t.staged = frame.transaction
		frame.transaction.states = append(frame.transaction.states, t)
	}

	blocks := make([]placed, len(t.blocks))
	top := t.start
	for i, current := range t.blocks {
		height := 0
		if width > 0 && current.block != nil {
			height = max(current.block.Measure(width), 0)
		}
		blocks[i] = placed{
			block: current.block, height: height, top: top, finished: current.finished,
		}
		top += height
	}
	t.pendingLayout = transcriptLayoutState{
		blocks: blocks,
		width:  width,
		rows:   top - t.start,
		start:  t.start,
		first:  t.first,
	}
	return TranscriptLayout{state: t.pendingLayout}
}

func (t *Transcript) commit(tx *transaction) {
	if t.staged != tx {
		return
	}
	t.blocks = t.pendingLayout.blocks
	t.width = t.pendingLayout.width
	t.rows = t.pendingLayout.rows
	t.pendingLayout = transcriptLayoutState{}
	t.staged = nil
}

func (t *Transcript) abort(tx *transaction) {
	if t.staged != tx {
		return
	}
	t.pendingLayout = transcriptLayoutState{}
	t.staged = nil
}

type transcriptLayoutState struct {
	blocks []placed
	width  int
	rows   int
	start  int
	first  BlockID
}

// TranscriptLayout is the immutable placement used to draw one transcript frame.
// It is returned by Transcript.Layout and Transcript.Stage.
type TranscriptLayout struct{ state transcriptLayoutState }

// Height is the number of live rows in this layout.
func (l TranscriptLayout) Height() int { return l.state.rows }

// StartRow is the first live row in this layout.
func (l TranscriptLayout) StartRow() int { return l.state.start }

// EndRow is the exclusive end of this layout's live rows.
func (l TranscriptLayout) EndRow() int { return layout.Sum(l.state.start, l.state.rows) }

// FirstBlock is the identity of the first live block.
func (l TranscriptLayout) FirstBlock() BlockID { return l.state.first }

// Block returns the live block with id.
func (l TranscriptLayout) Block(id BlockID) Block {
	i, ok := l.position(id)
	if !ok {
		return nil
	}
	return l.state.blocks[i].block
}

// Extent is the first row and height occupied by id.
func (l TranscriptLayout) Extent(id BlockID) (top, height int, ok bool) {
	i, ok := l.position(id)
	if !ok {
		return 0, 0, false
	}
	block := l.state.blocks[i]
	return block.top, block.height, true
}

// Draw writes the window of rows starting at from.
func (l TranscriptLayout) Draw(v grid.View, from int) {
	w, h := v.Size()
	if w <= 0 || h <= 0 {
		return
	}
	first, last := l.visible(from, h)
	for i := first; i < last; i++ {
		block := l.state.blocks[i]
		if block.height == 0 {
			continue
		}
		y := block.top - from
		block.block.Draw(v.Sub(grid.Rect(0, y, w, block.height)))
	}
}

func (l TranscriptLayout) position(id BlockID) (int, bool) {
	i := int(id - l.state.first)
	return i, id >= l.state.first && i >= 0 && i < len(l.state.blocks)
}

func (l TranscriptLayout) visible(from, rows int) (first, last int) {
	if rows <= 0 || l.state.rows == 0 || from >= l.EndRow() {
		return len(l.state.blocks), len(l.state.blocks)
	}
	if from < l.state.start {
		if l.state.start > 0 {
			rows -= l.state.start - from
		}
		from = l.state.start
	}
	if rows <= 0 {
		return len(l.state.blocks), len(l.state.blocks)
	}
	first = l.index(from)
	end := layout.Sum(from, rows)
	last = first
	for last < len(l.state.blocks) && l.state.blocks[last].top < end {
		last++
	}
	return first, last
}

func (l TranscriptLayout) index(row int) int {
	lo, hi := 0, len(l.state.blocks)-1
	for lo < hi {
		mid := (lo + hi + 1) / 2
		if l.state.blocks[mid].top <= row {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	return lo
}

// remeasure recomputes heights and tops from i onwards.
func (t *Transcript) remeasure(from int) {
	top := t.start
	if from > 0 {
		prev := t.blocks[from-1]
		top = layout.Sum(prev.top, prev.height)
	}
	for i := from; i < len(t.blocks); i++ {
		t.blocks[i].height = t.measure(t.blocks[i].block)
		t.blocks[i].top = top
		top = layout.Sum(top, t.blocks[i].height)
	}
	t.rows = layout.Remaining(top, t.start)
}

// measure is a block's height at the current width, never negative.
func (t *Transcript) measure(b Block) int {
	if t.width <= 0 {
		return 0
	}
	return max(b.Measure(t.width), 0)
}

// Extent is the rows block i covers: the first, and how many.
//
// It reports false for an identity that is not live, which is what a caller holding a
// reference from an earlier frame needs after a commit removes a prefix.
func (t *Transcript) Extent(id BlockID) (top, height int, ok bool) {
	i, ok := t.position(id)
	if !ok {
		return 0, 0, false
	}
	return t.blocks[i].top, t.blocks[i].height, true
}

// At is the block covering a row, and how far into that block the row is.
//
// The search is a bisection over the tops rather than a walk, because this is asked
// once per click, once per selection end, and once per frame for the top of the view.
// Deliberately retained content may be much taller than a screen.
func (t *Transcript) At(row int) (id BlockID, offset int, ok bool) {
	if row < t.start || row >= t.EndRow() {
		return 0, 0, false
	}
	i := t.index(row)
	return t.first + BlockID(i), row - t.blocks[i].top, true
}

// position resolves a stable identity into the current compact slice.
func (t *Transcript) position(id BlockID) (int, bool) {
	i := int(id - t.first)
	return i, id >= t.first && i >= 0 && i < len(t.blocks)
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
// The end is exclusive. An empty range comes back as two equal identities, which is
// what a viewport scrolled past everything gives.
func (t *Transcript) Visible(from, rows int) (first, last BlockID) {
	a, b := t.visible(from, rows)
	return t.first + BlockID(a), t.first + BlockID(b)
}

func (t *Transcript) visible(from, rows int) (first, last int) {
	if rows <= 0 || t.rows == 0 || from >= t.EndRow() {
		return len(t.blocks), len(t.blocks)
	}
	if from < t.start {
		if t.start > 0 {
			rows -= t.start - from
		}
		from = t.start
	}
	if rows <= 0 {
		return len(t.blocks), len(t.blocks)
	}
	first = t.index(from)
	end := layout.Sum(from, rows)
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
	first, last := t.visible(from, h)
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
// this contributes empty rows to a selection rather than nothing at all — it still
// occupies the rows, and a selection dragged across it has to come out with as many
// lines as the user dragged over.
type Copyable interface {
	// Rows is what the block's rows say at a width, and there are as many of them as
	// Measure reports at that width.
	Rows(width int) []text.Row
}

// Rows is what the transcript says over [from, from+count), one entry per row.
//
// Rows belonging to a block that cannot be copied come back empty and unjoined. The
// count is what was asked for, clamped to what exists, so a caller can index the
// result by row and get the row it meant.
func (t *Transcript) Rows(from, count int) []text.Row {
	if count <= 0 || from >= t.EndRow() {
		return nil
	}
	if from < t.start {
		if t.start > 0 {
			count -= t.start - from
		}
		from = t.start
	}
	count = min(count, t.EndRow()-from)
	if count <= 0 {
		return nil
	}
	out := make([]text.Row, count)

	first, last := t.visible(from, count)
	for i := first; i < last; i++ {
		b := t.blocks[i]
		if b.height == 0 {
			continue
		}
		copyable, ok := b.block.(Copyable)
		if !ok {
			continue
		}
		rows := copyable.Rows(t.width)
		for row := range b.height {
			at := b.top + row - from
			if at < 0 || at >= count || row >= len(rows) {
				continue
			}
			out[at] = rows[row]
		}
	}
	return out
}

// Finish says a block will not change again.
//
// It is what makes a block eligible to be given to the terminal, and it is the caller's
// to say: a streaming answer is finished when whatever is streaming it says so, and
// nothing here can tell a pause from an ending.
func (t *Transcript) Finish(id BlockID) {
	i, ok := t.position(id)
	if !ok {
		return
	}
	t.blocks[i].finished = true
}

// Finished reports whether a block has been said to be finished.
func (t *Transcript) Finished(id BlockID) bool {
	i, ok := t.position(id)
	return ok && t.blocks[i].finished
}

// Commit gives the leading run of finished blocks to the terminal, in order, and
// reports how many went.
//
// # Why only the leading run
//
// Text printed into a terminal's own output goes after what is already there, and
// there is no way to put something in front of it. So a block that finished while an
// earlier one is still being written has to wait: giving it over first would put the
// answer above the question.
//
// # Why this is one call
//
// The alternative is a range to ask for and a range to record afterwards, and the
// second half of that pair is the one that gets forgotten — which prints the whole
// session again on the next frame. give is called with each block and its height, and
// returning false stops the run and leaves that block and everything after it for
// another time.
//
// # It is a one-way door
//
// A committed block belongs to the terminal. It is no longer drawn, no longer
// re-wrapped when the window changes, and no longer selectable or searchable by this
// program — that is the trade printing makes, and it is why nothing is committed
// unless it is asked for. What it buys is that the output survives the program
// exiting, and that a session's memory stops growing.
func (t *Transcript) Commit(give func(b Block, rows int) bool) int {
	if give == nil {
		return 0
	}
	gone, rows := 0, 0
	for _, b := range t.blocks {
		if !b.finished {
			break
		}
		if !give(b.block, b.height) {
			break
		}
		gone++
		rows = layout.Sum(rows, b.height)
	}
	if gone == 0 {
		return 0
	}

	t.release(gone)
	t.first += BlockID(gone)
	t.start = layout.Sum(t.start, rows)
	t.rows = layout.Remaining(t.rows, rows)
	return gone
}

// release removes n leading placements while keeping backing storage proportional to
// the blocks that remain. Clearing drops payload references immediately. Reallocating
// only after the unused prefix is larger than the live suffix makes repeated commits
// amortized linear rather than copying the retained transcript on every block.
func (t *Transcript) release(n int) {
	clear(t.blocks[:n])
	t.blocks = t.blocks[n:]
	if len(t.blocks) == 0 {
		t.blocks = nil
		return
	}
	if cap(t.blocks) <= 2*len(t.blocks)+64 {
		return
	}
	blocks := make([]placed, len(t.blocks))
	copy(blocks, t.blocks)
	t.blocks = blocks
}
