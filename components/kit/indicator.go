package kit

import (
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/layout"
	"github.com/Tangerg/oolong/core/text"
)

// Spinner shows that something is happening.
//
// It holds a frame number rather than a clock. The caller decides when
// time passes, which is what lets a test step it deterministically and what keeps an
// idle UI from waking up to animate something nobody is waiting for.
type Spinner struct {
	// Theme is the look: the glyph is what the interface is waiting on, and the
	// label is a note about it.
	Theme Theme
	// Glyphs supplies the coverage-aware frames. A set with no frames draws the label
	// alone, following the package-wide rule that zero glyphs invent no furniture.
	Glyphs Glyphs
	// Label follows the glyph, if there is room.
	Label string

	frame int
}

// Tick advances the animation by one frame.
func (s *Spinner) Tick() { s.frame++ }

// Measure is one row.
func (s *Spinner) Measure(int) int { return 1 }

// Draw writes the current frame and its label.
func (s *Spinner) Draw(v grid.View) {
	w, h := v.Size()
	if w <= 0 || h <= 0 {
		return
	}
	x := 0
	frames := s.Glyphs.Spinner
	if len(frames) > 0 {
		glyph := frames[((s.frame%len(frames))+len(frames))%len(frames)]
		x = v.Text(0, 0, glyph, s.Theme.Accent)
		if x < w {
			x++
		}
	}
	if s.Label == "" || x >= w {
		return
	}
	v.Text(x, 0, text.Truncate(s.Label, w-x, s.Glyphs.Ellipsis), s.Theme.Muted)
}

// Scrollbar shows where a window sits in something taller than itself.
//
// It is drawn, not interacted with. Scrolling is the business of whatever owns the
// content and the keys; this only says where you are, which is the part a user
// cannot work out for themselves.
type Scrollbar struct {
	// Total is how many rows of content there are, Window how many are shown, and
	// Offset how many are above the window.
	Total, Window, Offset int
	// Theme is the look: the track is structure the eye should skip, and the thumb
	// is where you are.
	Theme Theme
	// Glyphs supplies the track and thumb. A zero set draws nothing, following the
	// same coverage rule as every other piece of kit furniture.
	Glyphs Glyphs
}

// Needed reports whether there is anything to indicate. A bar drawn when everything
// already fits is a column of decoration.
func (s Scrollbar) Needed() bool { return s.Total > s.Window && s.Window > 0 }

// Draw paints the bar down the first column of v.
func (s Scrollbar) Draw(v grid.View) {
	_, h := v.Size()
	if h <= 0 {
		return
	}
	track, thumb := s.Glyphs.ScrollTrack, s.Glyphs.ScrollThumb
	if !s.Needed() {
		if track != "" {
			for y := range h {
				v.Text(0, y, track, s.Theme.Subtle)
			}
		}
		return
	}

	// At least one row of thumb, however long the content: a thumb rounded down to
	// nothing tells the user nothing.
	size := max(layout.Scale(h, s.Window, s.Total), 1)
	scrollable := s.Total - s.Window
	span := h - size
	top := 0
	if scrollable > 0 && span > 0 {
		top = layout.Scale(span, s.Offset, scrollable)
	}
	for y := range h {
		if y >= top && y < top+size {
			if thumb != "" {
				v.Text(0, y, thumb, s.Theme.Muted)
			}
			continue
		}
		if track != "" {
			v.Text(0, y, track, s.Theme.Subtle)
		}
	}
}
