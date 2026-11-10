package base

import (
	"sort"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/text"
)

type placed struct {
	revision    uint64
	mapping     *transcriptTextMap
	id          BlockID
	block       Block
	height, top int
}

// TranscriptLayout is immutable geometry and text mapping for one complete scene candidate.
type TranscriptLayout struct {
	key         transcriptLayoutKey
	blocks      []placed
	rows, start int
	first       BlockID
}

type transcriptLayoutKey struct {
	owner    *byte
	revision uint64
	width    int
}

// Width is the column count used by every placement in this projection.
func (l TranscriptLayout) Width() int { return l.key.width }

// Height is the total retained row span in this projection.
func (l TranscriptLayout) Height() int { return l.rows }

// StartRow is the inclusive origin shared by drawing, selection and hit testing.
func (l TranscriptLayout) StartRow() int { return l.start }

// EndRow is the exclusive end of the checked row span.
func (l TranscriptLayout) EndRow() int { return l.start + l.rows }

// FirstBlock identifies the first retained content at preparation time.
func (l TranscriptLayout) FirstBlock() BlockID { return l.first }

// Block resolves the passive content held by this immutable layout, including earlier committed frames.
func (l TranscriptLayout) Block(id BlockID) Block {
	b, _ := l.placed(id)
	return b.block
}

// Extent reads the placement in this immutable frame.
func (l TranscriptLayout) Extent(id BlockID) (top, height int, ok bool) {
	b, ok := l.placed(id)
	return b.top, b.height, ok
}

func (l TranscriptLayout) window(from, count int) (int, int, bool) {
	if count <= 0 || l.rows == 0 {
		return 0, 0, false
	}
	end := min(layout.Translate(from, count), l.EndRow())
	from = max(from, l.start)
	return from, max(end-from, 0), end > from
}

func (l TranscriptLayout) visible(from, rows int) (first, last int) {
	from, rows, ok := l.window(from, rows)
	if !ok {
		return len(l.blocks), len(l.blocks)
	}
	first = sort.Search(len(l.blocks), func(i int) bool { return l.blocks[i].top+l.blocks[i].height > from })
	last = first
	end := from + rows
	for last < len(l.blocks) && l.blocks[last].top < end {
		last++
	}
	return first, last
}

// At resolves a row to its content identity and local row offset, skipping empty placements.
func (l TranscriptLayout) At(row int) (BlockID, int, bool) {
	first, last := l.visible(row, 1)
	if first == last {
		return 0, 0, false
	}
	b := l.blocks[first]
	return b.id, row - b.top, true
}

func (l TranscriptLayout) Draw(view grid.View, from int) {
	width, _ := view.Size()
	visible := view.Visible()
	if visible.Empty() {
		return
	}
	first, last := l.visible(layout.Translate(from, visible.Min.Y), visible.Dy())
	for _, b := range l.blocks[first:last] {
		if b.block != nil && b.height > 0 {
			b.block.Draw(view.Sub(grid.Area(0, b.top-from, width, b.height)))
		}
	}
}

// Rows returns a bounded physical row window, preserving empty slots for content without text.
func (l TranscriptLayout) Rows(from, count int) []text.Row {
	from, count, ok := l.window(from, count)
	if !ok {
		return nil
	}
	out := make([]text.Row, count)
	first, last := l.visible(from, count)
	for _, b := range l.blocks[first:last] {
		if b.mapping == nil {
			continue
		}
		source := min(max(from-b.top, 0), len(b.mapping.rows))
		destination := max(b.top-from, 0)
		n := min(len(out)-destination, len(b.mapping.rows)-source)
		copy(out[destination:], text.CloneRows(b.mapping.rows[source:source+n]))
	}
	return out
}

// Visible returns the half-open identity span touched by a row window.
func (l TranscriptLayout) Visible(from, rows int) (BlockID, BlockID) {
	a, b := l.visible(from, rows)
	first := l.first + BlockID(len(l.blocks))
	if a < len(l.blocks) {
		first = l.blocks[a].id
	}
	last := first
	if b > a {
		last = l.blocks[b-1].id + 1
	}
	return first, last
}
