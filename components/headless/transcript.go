package headless

import (
	"math"

	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/layout"
	"github.com/Tangerg/oolong/core/text"
)

// Transcript owns retained blocks and their shared row coordinates for scrolling,
// selection and search. Append and tail growth are constant time; width changes
// recompute heights. The zero value has width zero; Stage establishes committed width.
// A Transcript must not be copied after first use. Commit callbacks may only read it;
// mutations and Stage panic during unsettled publication, including abnormal exits.
type Transcript struct {
	releasedPrefix int
	noCopy         noCopy
	publishing     bool

	// transcriptState is embedded because these are the transcript's committed
	// values, not a cache beside it. A frame derives another complete state and swaps
	// it in atomically; keeping both as the same entity prevents commit and Layout
	// from becoming two hand-maintained field lists.
	transcriptState

	pendingLayout transcriptState
	staged        *transaction
}

// BlockID is the stable identity of one block in a [Transcript].
//
// IDs increase as blocks are appended and are never reused. Committing a leading
// block invalidates its ID without changing the IDs of blocks that remain live. That
// is what lets a sticky header or another retained part keep referring to a live block
// while committed storage is physically removed from the transcript. The maximum
// value is reserved as the exhausted next identity; reaching it panics before an old
// identity could be reused.
type BlockID uint64

const exhaustedBlockID = ^BlockID(0)

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
func (t *Transcript) EndRow() int { return t.endRow() }

// FirstBlock is the ID of the first live block. When the transcript is empty it is
// the ID the next appended block will receive.
func (t *Transcript) FirstBlock() BlockID { return t.first }

// Width is the width every height in it was measured at.
func (t *Transcript) Width() int { return t.width }

// Append adds a block at the end, measures it, and returns its stable identity.
// Appending nil changes nothing and returns the next available identity.
//
// Identities are never reused, and Append panics once every one has been issued.
// A session that wrapped would let a [BlockID] a caller is holding — a scroll
// anchor, a search result, a selection — name a different block than the one it was
// taken from, so the interface would jump somewhere the user never asked to go.
func (t *Transcript) Append(b Block) BlockID {
	t.assertMutable()
	live := BlockID(len(t.blocks))
	if live >= exhaustedBlockID-t.first {
		panic("headless: transcript exhausted block identities")
	}
	id := t.first + live
	if b == nil {
		return id
	}
	height := t.measure(b)
	if len(t.blocks) == cap(t.blocks) {
		t.releasedPrefix = 0
	}
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
	t.assertMutable()
	i, ok := t.position(id)
	if !ok {
		return
	}
	t.remeasure(i)
}

func (t *Transcript) layout() TranscriptLayout {
	if t == nil {
		return TranscriptLayout{}
	}
	return TranscriptLayout{state: t.transcriptState}
}

// Stage lays the transcript out at width for a component frame.
//
// A changed width is measured into private pending placement. The new row space
// becomes observable through Height, Extent, selection and search only when
// the complete Root frame commits. Calling Stage at the committed width reuses the
// existing placement without allocation.
func (t *Transcript) Stage(frame Frame, width int) TranscriptLayout {
	if t == nil {
		return TranscriptLayout{}
	}
	t.assertMutable()
	width = max(width, 0)
	if t.staged == nil && width == t.width {
		return t.layout()
	}
	frame.enlist(t, &t.staged)

	blocks := make([]placed, len(t.blocks))
	top := t.start
	for i, current := range t.blocks {
		height := 0
		if width > 0 && current.block != nil {
			height = max(current.block.HeightForWidth(width), 0)
		}
		blocks[i] = placed{
			block: current.block, height: height, top: top, finished: current.finished,
		}
		top = layout.Sum(top, height)
	}
	t.pendingLayout = transcriptState{
		blocks: blocks,
		width:  width,
		rows:   layout.Remaining(top, t.start),
		start:  t.start,
		first:  t.first,
	}
	return TranscriptLayout{state: t.pendingLayout}
}

func (t *Transcript) commit(tx *transaction) {
	if t.staged != tx {
		return
	}
	t.transcriptState = t.pendingLayout
	t.pendingLayout = transcriptState{}
	t.staged = nil
}

func (t *Transcript) abort(tx *transaction) {
	if t.staged != tx {
		return
	}
	t.pendingLayout = transcriptState{}
	t.staged = nil
}

type transcriptState struct {
	blocks []placed
	width  int
	rows   int
	start  int
	first  BlockID
}

func (s transcriptState) endRow() int { return layout.Sum(s.start, s.rows) }

func (s transcriptState) position(id BlockID) (int, bool) {
	return blockPosition(s.first, id, len(s.blocks))
}

// index is the block covering row, which must be a row that exists.
//
// The last block whose top is at or below the row is the answer. A zero-height
// block shares its top with the block after it, so choosing the last such block also
// skips every zero-height block that cannot cover the row. A trailing zero-height
// run starts at endRow, which callers reject before asking.
func (s transcriptState) index(row int) int {
	lo, hi := 0, len(s.blocks)-1
	for lo < hi {
		mid := (lo + hi + 1) / 2
		if s.blocks[mid].top <= row {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	return lo
}

func (s transcriptState) visible(from, rows int) (first, last int) {
	from, rows, ok := s.window(from, rows)
	if !ok {
		return len(s.blocks), len(s.blocks)
	}
	first = s.index(from)
	end := layout.Sum(from, rows)
	last = first
	for last < len(s.blocks) && s.blocks[last].top < end {
		last++
	}
	return first, last
}

// window intersects an absolute row interval with the rows this state owns.
// Translate is the signed, saturating addition: a negative start remains meaningful
// and an enormous count cannot wrap the requested end behind it.
func (s transcriptState) window(from, rows int) (start, count int, ok bool) {
	if rows <= 0 || s.rows == 0 {
		return 0, 0, false
	}
	end := min(layout.Translate(from, rows), s.endRow())
	start = max(from, s.start)
	if end <= start {
		return 0, 0, false
	}
	return start, end - start, true
}

// TranscriptLayout is the immutable placement returned by [Transcript.Stage] for one
// component frame.
type TranscriptLayout struct{ state transcriptState }

// Height is the number of live rows in this layout.
func (l TranscriptLayout) Height() int { return l.state.rows }

// StartRow is the first live row in this layout.
func (l TranscriptLayout) StartRow() int { return l.state.start }

// EndRow is the exclusive end of this layout's live rows.
func (l TranscriptLayout) EndRow() int { return l.state.endRow() }

// FirstBlock is the identity of the first live block.
func (l TranscriptLayout) FirstBlock() BlockID { return l.state.first }

// Block returns the live block with id.
func (l TranscriptLayout) Block(id BlockID) Block {
	i, ok := l.state.position(id)
	if !ok {
		return nil
	}
	return l.state.blocks[i].block
}

// Extent is the first row and height occupied by id.
func (l TranscriptLayout) Extent(id BlockID) (top, height int, ok bool) {
	i, ok := l.state.position(id)
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
	first, last := l.state.visible(from, h)
	for i := first; i < last; i++ {
		block := l.state.blocks[i]
		if block.height == 0 {
			continue
		}
		y := block.top - from
		block.block.Draw(v.Sub(grid.Area(0, y, w, block.height)))
	}
}

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

func (t *Transcript) measure(b Block) int {
	if t.width <= 0 {
		return 0
	}
	return max(b.HeightForWidth(t.width), 0)
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
	return t.first + blockOffset(i), row - t.blocks[i].top, true
}

// blockPosition resolves an unsigned stable identity only after proving its distance
// fits the architecture's slice index. Comparing before subtracting is what keeps a
// stale earlier identity from wrapping into the live range.
func blockPosition(first, id BlockID, count int) (int, bool) {
	if id < first {
		return 0, false
	}
	offset := id - first
	if offset > BlockID(math.MaxInt) {
		return 0, false
	}
	i := int(offset)
	return i, i < count
}

// blockOffset converts a proven slice position into an identity offset. Keeping the
// sign check here makes the conversion honest even if a future caller stops getting
// its position directly from a slice operation.
func blockOffset(index int) BlockID {
	if index < 0 {
		panic("headless: negative transcript block position")
	}
	return BlockID(index)
}

// Visible is the range of blocks that any of the rows [from, from+rows) touch.
//
// The end is exclusive. An empty range comes back as two equal identities, which is
// what a viewport scrolled past everything gives.
func (t *Transcript) Visible(from, rows int) (first, last BlockID) {
	a, b := t.visible(from, rows)
	return t.first + blockOffset(a), t.first + blockOffset(b)
}

// TextProjector exposes meaningful text separately from painted decoration. Blocks
// without it still contribute empty rows so selection preserves physical row count.
type TextProjector interface {
	// Rows is what the block's rows say at a width, and there are as many of them as
	// HeightForWidth reports at that width.
	Rows(width int) []text.Row
}

// Rows is what the transcript says over [from, from+count), one entry per row.
//
// Rows belonging to a block that cannot project text come back empty and unjoined.
// The count is what was asked for, clamped to what exists, so a caller can index the
// result by row and get the row it meant.
func (t *Transcript) Rows(from, count int) []text.Row {
	from, count, ok := t.window(from, count)
	if !ok {
		return nil
	}
	out := make([]text.Row, count)

	first, last := t.visible(from, count)
	for i := first; i < last; i++ {
		b := t.blocks[i]
		if b.height == 0 {
			continue
		}
		projector, ok := b.block.(TextProjector)
		if !ok {
			continue
		}
		rows := projector.Rows(t.width)
		for row := range b.height {
			at := layout.Relative(layout.Sum(b.top, row), from)
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
	t.assertMutable()
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

// Commit transfers the finished leading blocks in order and returns the count. Each
// accepted block retires before the next callback. give receives its height; false
// retains that block and the suffix. Only the leading run can transfer without reordering output.
// Transferred blocks leave program-controlled wrapping, selection and search.
// Callbacks may only read; reentrant mutation or publication panics. Abnormal exits
// leave publication unsettled and readable, with further mutations panicking.
func (t *Transcript) Commit(give func(b Block, rows int) bool) int {
	t.assertMutable()
	if give == nil {
		return 0
	}
	t.publishing = true
	gone := 0
	for len(t.blocks) > 0 {
		b := t.blocks[0]
		if !b.finished {
			break
		}
		if !give(b.block, b.height) {
			break
		}
		gone++
		t.blocks[0] = placed{}
		t.blocks = t.blocks[1:]
		t.releasedPrefix++
		t.first++
		t.start = layout.Sum(t.start, b.height)
		t.rows = layout.Remaining(t.rows, b.height)
	}
	t.compact()
	t.publishing = false
	return gone
}

func (t *Transcript) assertMutable() {
	if t.publishing {
		panic("headless: transcript publication is unsettled")
	}
}

// Compact once after publication, so all retired slots are cleared before moving the
// live suffix. The threshold keeps repeated commits amortized linear.
func (t *Transcript) compact() {
	allocation := cap(t.blocks) + t.releasedPrefix
	if len(t.blocks) == 0 {
		t.releasedPrefix = 0
		t.blocks = nil
		return
	}
	if allocation <= 2*len(t.blocks)+64 {
		return
	}
	blocks := make([]placed, len(t.blocks))
	copy(blocks, t.blocks)
	t.blocks = blocks
	t.releasedPrefix = 0
}
