package base

import (
	"slices"

	"github.com/Tangerg/oolong/ui/core/anim"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/internal/fifo"
)

// Sticky pins a block to the top of the view once it has been scrolled past.
//
// The thing that gives the rows below their meaning — which question this answer
// belongs to — is exactly the thing that scrolls away first.
//
// The pinned block sits at the top of the view while its own rows are above it. The
// next pinned block pushes it off rather than appearing over it, so the change reads
// as one thing replacing another instead of two things flickering.
//
// All of it is arithmetic about rows — which block is pinned, how many of its rows are
// left, how far along the push is — so none of it draws.
//
// The zero value has no pinnable blocks. A Sticky must not be copied after first
// use; its ordered identity set has one mutable owner.
type Sticky struct {
	noCopy  noCopy
	element scene.Element

	// blocks are the identities of the blocks that can be pinned, in order. They are
	// private because discarding a committed prefix mutates this collection; retaining
	// a caller-owned slice would clear the caller's data while reclaiming ours.
	//
	// Only the caller knows which blocks mean anything: in a session the prompts are
	// worth pinning and the answers are not, and nothing here can tell one from the
	// other.
	blocks fifo.Queue[BlockID]
	// MinHeight is how far a header may be collapsed before it stops shrinking and
	// starts scrolling off instead. Zero means it does not collapse.
	//
	// A tall prompt pinned in full eats the view it was meant to give context to.
	minHeight int
	// Gap is the rows kept clear between a pinned header and the content below it,
	// so that the two do not read as one block.
	gap int
}

type StickyConfig struct {
	Blocks    []BlockID
	MinHeight int
	Gap       int
}

// NewSticky panics for negative dimensions or block identities that are not
// nonzero and strictly increasing. The caller retains ownership of config.Blocks.
func NewSticky(config StickyConfig) *Sticky {
	if config.MinHeight < 0 || config.Gap < 0 {
		panic("base: sticky dimensions cannot be negative")
	}
	s := &Sticky{minHeight: config.MinHeight, gap: config.Gap}
	s.SetBlocks(config.Blocks)
	return s
}

// SetBlocks replaces the identities that can be pinned. It panics before mutation
// unless identities are nonzero and strictly increasing in transcript order.
// Sticky owns the slice; the caller may reuse or change its input afterwards.
func (s *Sticky) SetBlocks(blocks []BlockID) {
	if s.setBlocks(blocks) {
		s.element.Invalidate()
	}
}

func (s *Sticky) setBlocks(blocks []BlockID) bool {
	requireMutable(s.element)
	requireStickyOrder(blocks, 0)
	if s.blocks.Len() == len(blocks) {
		equal := true
		for i, id := range blocks {
			if s.blocks.At(i) != id {
				equal = false
				break
			}
		}
		if equal {
			return false
		}
	}
	s.blocks.Clear()
	s.blocks.Push(blocks...)
	return true
}

// Add appends pinnable block identities in transcript order. It panics before
// mutation unless every identity strictly follows the existing set and its predecessor.
func (s *Sticky) Add(blocks ...BlockID) {
	requireMutable(s.element)
	var last BlockID
	if s.blocks.Len() > 0 {
		last = s.blocks.At(s.blocks.Len() - 1)
	}
	requireStickyOrder(blocks, last)
	if len(blocks) > 0 {
		s.blocks.Push(blocks...)
		s.element.Invalidate()
	}
}

func requireStickyOrder(blocks []BlockID, after BlockID) {
	for _, id := range blocks {
		if id <= after {
			panic("base: sticky block identities must be nonzero and strictly increasing")
		}
		after = id
	}
}

// Blocks returns a copy of the pinnable identities in transcript order.
func (s *Sticky) Blocks() []BlockID {
	if s == nil {
		return nil
	}
	return slices.Collect(s.blocks.Values())
}

func (s *Sticky) Len() int {
	if s == nil {
		return 0
	}
	return s.blocks.Len()
}

// Pinned is the header for one frame.
type Pinned struct {
	// Block is the identity of the block being pinned.
	Block   BlockID
	Height  int
	ClipTop int
	// Rows is the whole footprint at the top of the view: the visible header and the
	// gap under it. It is what a caller subtracts before drawing the content.
	Rows int
	// Fade is one for a header sitting still and falls towards zero as the next one
	// pushes it off, for blending it towards the background.
	Fade float64
}

func (p Pinned) Visible() int { return max(p.Height-p.ClipTop, 0) }

// At works out the header for a view of rows starting at from, and reports whether
// there is one.
//
// There is none while the block that would be pinned is still fully on screen: a
// header repeating something already visible two rows below is noise, and the moment
// it stops being visible is exactly the moment it starts being worth showing.
func (s *Sticky) At(t TranscriptLayout, from, rows int) (Pinned, bool) {
	if rows <= 0 || s.blocks.Len() == 0 ||
		from < t.StartRow() || from >= t.EndRow() {
		// A view scrolled past everything has nothing below the header for the header
		// to give context to.
		return Pinned{}, false
	}
	i, ok := s.pinnedAt(t, from)
	if !ok {
		return Pinned{}, false
	}
	top, height, ok := t.Extent(i)
	if !ok || height == 0 {
		return Pinned{}, false
	}
	if top >= from {
		return Pinned{}, false
	}

	p := Pinned{Block: i, Height: height}
	if s.minHeight > 0 {
		// Collapsed towards the floor as its own rows scroll away, so a tall prompt
		// does not go on eating the view it was meant to explain.
		p.Height = max(min(height, height-(from-top)), s.minHeight)
		p.Height = min(p.Height, height)
	}
	p.Rows = layout.Sum(p.Height, s.gap)
	p.Fade = 1

	if next, ok := s.nextAfter(t, i); ok {
		if room := next - from; room < p.Rows {
			p.ClipTop = min(p.Rows-max(room, 0), p.Height)
			p.Rows = max(room, 0)
			if p.Height > 0 {
				p.Fade = anim.EaseOutCubic(float64(p.Visible()) / float64(p.Height))
			}
		}
	}
	if p.Rows <= 0 {
		return Pinned{}, false
	}
	return p, true
}

func (s *Sticky) pinnedAt(t TranscriptLayout, row int) (BlockID, bool) {
	var found BlockID
	ok := false
	for id := range s.blocks.Values() {
		top, _, exists := t.Extent(id)
		if !exists {
			continue
		}
		if top > row {
			break
		}
		found, ok = id, true
	}
	return found, ok
}

func (s *Sticky) nextAfter(t TranscriptLayout, id BlockID) (int, bool) {
	for candidate := range s.blocks.Values() {
		if candidate <= id {
			continue
		}
		if top, _, ok := t.Extent(candidate); ok {
			return top, true
		}
	}
	return 0, false
}

// DiscardBefore forgets pinnable blocks that can no longer be addressed.
//
// A transcript calls for this after committing a prefix. Keeping one scalar identity
// per terminal-owned block would otherwise make the sticky state grow with session
// age even though the transcript itself had released the payload.
func (s *Sticky) DiscardBefore(first BlockID) {
	if s.discardBefore(first) {
		s.element.Invalidate()
	}
}

func (s *Sticky) discardBefore(first BlockID) bool {
	requireMutable(s.element)
	n := 0
	for n < s.blocks.Len() && s.blocks.At(n) < first {
		n++
	}
	s.blocks.Discard(n)
	return n > 0
}

func (s *Sticky) MinHeight() int { return s.minHeight }

// SetMinHeight panics for a negative minimum. Zero keeps the full header height.
func (s *Sticky) SetMinHeight(value int) {
	element := s.element
	requireMutable(element)
	if value < 0 {
		panic("base: MinHeight cannot be negative")
	}
	if s.minHeight == value {
		return
	}
	s.minHeight = value
	element.Invalidate()
}

func (s *Sticky) Gap() int { return s.gap }

// SetGap panics for a negative gap before changing header geometry.
func (s *Sticky) SetGap(value int) {
	element := s.element
	requireMutable(element)
	if value < 0 {
		panic("base: Gap cannot be negative")
	}
	if s.gap == value {
		return
	}
	s.gap = value
	element.Invalidate()
}
