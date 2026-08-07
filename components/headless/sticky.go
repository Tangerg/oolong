package headless

import (
	"slices"

	"github.com/Tangerg/oolong/core/anim"
)

// Sticky pins a block to the top of the view once it has been scrolled past.
//
// It is the section header of a scrolling list, and a transcript wants it for the same
// reason a list does: the thing that gives the rows below their meaning — which
// question this answer belongs to — is exactly the thing that scrolls away first. A
// reader halfway down a long answer has nothing on screen telling them what it
// answers.
//
// # How it behaves
//
// The pinned block sits at the top of the view while its own rows are above it. When
// the next pinned block comes up from below, it pushes this one off rather than
// appearing over it: the header slides up, is clipped from the top, and fades as it
// goes. That is what makes the change feel like one thing replacing another instead of
// two things flickering.
//
// # Why it is arithmetic and not drawing
//
// All of it is a question about rows: which block is pinned, how many of its rows are
// left, and how far along the push is. None of that needs a cell, so none of it is
// here. What draws a header takes these numbers and draws.
type Sticky struct {
	// blocks are the identities of the blocks that can be pinned, in order. They are
	// private because discarding a committed prefix mutates this collection; retaining
	// a caller-owned slice would clear the caller's data while reclaiming ours.
	//
	// Only the caller knows which blocks mean anything: in a session the prompts are
	// worth pinning and the answers are not, and nothing here can tell one from the
	// other.
	blocks []BlockID
	// MinHeight is how far a header may be collapsed before it stops shrinking and
	// starts scrolling off instead. Zero means it does not collapse.
	//
	// A tall prompt pinned in full eats the view it was meant to give context to.
	MinHeight int
	// Gap is the rows kept clear between a pinned header and the content below it,
	// so that the two do not read as one block.
	Gap int
}

// SetBlocks replaces the identities that can be pinned. Sticky owns the slice; the
// caller may reuse or change its input afterwards.
func (s *Sticky) SetBlocks(blocks []BlockID) { s.blocks = own(s.blocks, blocks) }

// Add appends pinnable block identities in transcript order.
func (s *Sticky) Add(blocks ...BlockID) { s.blocks = append(s.blocks, blocks...) }

// Blocks returns a copy of the pinnable identities in transcript order.
func (s *Sticky) Blocks() []BlockID {
	if s == nil {
		return nil
	}
	return slices.Clone(s.blocks)
}

// Len reports how many pinnable identities Sticky owns.
func (s *Sticky) Len() int {
	if s == nil {
		return 0
	}
	return len(s.blocks)
}

// Pinned is the header for one frame.
type Pinned struct {
	// Block is the identity of the block being pinned.
	Block BlockID
	// Height is how many of its rows to draw, which is fewer than it has when it is
	// collapsed or being pushed off.
	Height int
	// ClipTop is how many of its rows to leave off the top, which is what being
	// pushed off looks like.
	ClipTop int
	// Rows is the whole footprint at the top of the view: the visible header and the
	// gap under it. It is what a caller subtracts before drawing the content.
	Rows int
	// Fade is one for a header sitting still and falls towards zero as the next one
	// pushes it off, for blending it towards the background.
	Fade float64
}

// Visible is how many of the header's rows are drawn.
func (p Pinned) Visible() int { return max(p.Height-p.ClipTop, 0) }

// At works out the header for a view of rows starting at from, and reports whether
// there is one.
//
// There is none while the block that would be pinned is still fully on screen: a
// header repeating something already visible two rows below is noise, and the moment
// it stops being visible is exactly the moment it starts being worth showing.
func (s *Sticky) At(t TranscriptLayout, from, rows int) (Pinned, bool) {
	if rows <= 0 || len(s.blocks) == 0 ||
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
		return Pinned{}, false // still on screen where it belongs
	}

	p := Pinned{Block: i, Height: height}
	if s.MinHeight > 0 {
		// Collapsed towards the floor as its own rows scroll away, so a tall prompt
		// does not go on eating the view it was meant to explain.
		p.Height = max(min(height, height-(from-top)), s.MinHeight)
		p.Height = min(p.Height, height)
	}
	p.Rows = p.Height + s.Gap
	p.Fade = 1

	// The next pinned block pushes this one off as it comes up.
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

// pinnedAt is the last pinnable block at or above a row.
func (s *Sticky) pinnedAt(t TranscriptLayout, row int) (BlockID, bool) {
	var found BlockID
	ok := false
	for _, id := range s.blocks {
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

// nextAfter is the row the next pinnable block after i begins on.
func (s *Sticky) nextAfter(t TranscriptLayout, id BlockID) (int, bool) {
	for _, candidate := range s.blocks {
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
	n := 0
	for n < len(s.blocks) && s.blocks[n] < first {
		n++
	}
	if n == 0 {
		return
	}
	clear(s.blocks[:n])
	s.blocks = s.blocks[n:]
	s.blocks = trim(s.blocks)
}
