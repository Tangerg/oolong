package kit

import (
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/text"
)

// Status is the line that says something is happening: a spinner, what it is doing,
// and how long it has been at it.
//
// It holds no clock. The caller advances it with [Status.Tick], which is
// what lets a test step it deterministically and what keeps an idle interface from
// waking up to animate something nobody is waiting for.
type Status struct {
	Theme  Theme
	Glyphs Glyphs
	Doing  string
	// Elapsed is shown after the label, receding. Empty shows nothing, which is right
	// until something has taken long enough to be worth reporting.
	Elapsed string

	frame int
}

func (s *Status) Tick() { s.frame++ }

func (s *Status) HeightForWidth(int) int { return 1 }

func (s *Status) Draw(v grid.View) {
	width, height := v.Size()
	if width <= 0 || height <= 0 {
		return
	}
	spinner := Spinner{Theme: s.Theme, Glyphs: s.Glyphs, Label: s.Doing, frame: s.frame}

	if s.Elapsed == "" {
		spinner.Draw(v)
		return
	}
	// The elapsed time is pinned to the right and the label gets what is left, so a
	// long label is what gets truncated and the number stays readable.
	elapsed := text.Of(s.Elapsed, s.Theme.Subtle)
	used := elapsed.Width()
	if used+2 >= width {
		spinner.Draw(v)
		return
	}
	spinner.Draw(v.Sub(grid.Area(0, 0, width-used-1, 1)))
	elapsed.Draw(v, width-used, 0)
}
