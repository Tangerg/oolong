package kit

import (
	"strings"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/text"
)

// Spinner shows that something is happening.
//
// It holds a frame number rather than a clock. The caller decides when
// time passes, which is what lets a test step it deterministically and what keeps an
// idle UI from waking up to animate something nobody is waiting for.
type Spinner struct {
	Theme  Theme
	Glyphs Glyphs
	Label  string

	frame int
}

// spinnerLabelGap separates the spinner glyph from its label.
const spinnerLabelGap = 1

func (s *Spinner) Tick() { s.frame++ }

func (s *Spinner) HeightForWidth(int) int { return 1 }

func (s *Spinner) Draw(v grid.View) {
	w, h := v.Size()
	if w <= 0 || h <= 0 {
		return
	}
	s.line().Truncate(w, s.Glyphs.Ellipsis).Draw(v, 0, 0)
}

// line is the spinner's row: the current frame's glyph, then the label.
func (s *Spinner) line() text.Line {
	var line text.Line
	if frames := s.Glyphs.Spinner; len(frames) > 0 {
		glyph := frames[((s.frame%len(frames))+len(frames))%len(frames)]
		line = append(line, text.Of(glyph, s.Theme.Accent)...)
		if s.Label != "" {
			line = append(line, text.Of(strings.Repeat(" ", spinnerLabelGap), s.Theme.Accent)...)
		}
	}
	return append(line, text.Of(s.Label, s.Theme.Muted)...)
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
	Theme                 Theme
	Glyphs                Glyphs
}

// Needed reports whether there is anything to indicate. A bar drawn when everything
// already fits is a column of decoration.
func (s Scrollbar) Needed() bool { return s.Total > s.Window && s.Window > 0 }

func (s Scrollbar) Draw(v grid.View) {
	_, h := v.Size()
	visible := v.Visible()
	if visible.Empty() {
		return
	}
	top, size := s.thumb(h)
	for y := visible.Min.Y; y < visible.Max.Y; y++ {
		glyph, style := s.Glyphs.ScrollTrack.value, s.Theme.Subtle
		if y >= top && y < layout.Sum(top, size) {
			glyph, style = s.Glyphs.ScrollThumb.value, s.Theme.Muted
		}
		if glyph != "" {
			v.Text(0, y, glyph, style)
		}
	}
}

// thumb is where the thumb starts on a bar of height rows and how many it takes,
// zero when there is nothing to indicate. A needed thumb takes at least one row: one
// rounded down to nothing tells the user nothing.
func (s Scrollbar) thumb(height int) (top, size int) {
	if !s.Needed() {
		return 0, 0
	}
	size = max(layout.Scale(height, s.Window, s.Total), 1)
	if scrollable, span := s.Total-s.Window, height-size; scrollable > 0 && span > 0 {
		top = layout.Scale(span, s.Offset, scrollable)
	}
	return top, size
}
