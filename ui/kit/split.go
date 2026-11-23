package kit

import (
	"slices"
	"strings"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/text"
)

// Split is one row holding Start at its left and End pinned whole against its
// right edge. End is what a reader needs complete — a count, a duration, a state —
// so when the row cannot hold both with Gap between them End is dropped rather than
// cut, and Start, which truncates with Ellipsis, takes the whole row.
type Split struct {
	Start, End text.Line
	// Gap separates Start from End. Zero uses one column; a negative gap panics.
	Gap      int
	Ellipsis string
}

var _ text.Projector = Split{}

func (Split) HeightForWidth(int) int { return 1 }

// Draw panics for a negative Gap before changing cells, even for an empty view.
func (s Split) Draw(v grid.View) {
	gap := s.gap()
	width, height := v.Size()
	if width <= 0 || height <= 0 {
		return
	}
	room, end := s.layout(width, gap)
	s.Start.Truncate(room, s.Ellipsis).Draw(v, 0, 0)
	if end > 0 {
		s.End.Draw(v, width-end, 0)
	}
}

// Rows projects Start whole and End where Draw pins it, so a selection over the
// row names the complete text rather than what truncation left. It panics for a
// negative Gap, like Draw.
func (s Split) Rows(width int) []text.Row {
	gap := s.gap()
	start := s.Start.Display()
	_, end := s.layout(max(width, 0), gap)
	if end == 0 {
		return []text.Row{{Content: start}}
	}
	padding := text.Of(strings.Repeat(" ", max(width-end-start.Width(), gap)), grid.Style{})
	return []text.Row{{Content: slices.Concat(start, padding, s.End.Display())}}
}

// layout is how many columns Start may take and how many End occupies, zero when
// the row cannot hold End beside at least one column of Start.
func (s Split) layout(width, gap int) (room, end int) {
	end = s.End.Width()
	if end == 0 || layout.Sum(end, gap, 1) > width {
		return width, 0
	}
	return width - end - gap, end
}

func (s Split) gap() int {
	if s.Gap < 0 {
		panic("kit: Split Gap must be non-negative")
	}
	return max(s.Gap, 1)
}
