package kit

import (
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/text"
)

// Status is the line that says something is happening: a spinner, what it is doing,
// and how long it has been at it.
//
// It holds no clock. The caller advances it with [Status.Tick], which is
// what lets a test step it deterministically and what keeps an idle interface from
// waking up to animate something nobody is waiting for.
type Status struct {
	Theme Theme
	// Doing is what is happening, in the present tense — "thinking", "reading
	// main.go", "running tests".
	Doing string
	// Elapsed is shown after the label, receding. Empty shows nothing, which is right
	// until something has taken long enough to be worth reporting.
	Elapsed string

	spinner Spinner
}

// Tick advances the spinner by one frame.
func (s *Status) Tick() { s.spinner.Tick() }

// Measure is one row.
func (s *Status) Measure(int) int { return 1 }

// Draw paints the spinner, the label and the elapsed time.
func (s *Status) Draw(v grid.View) {
	width, height := v.Size()
	if width <= 0 || height <= 0 {
		return
	}
	s.spinner.Theme = s.Theme
	s.spinner.Label = s.Doing

	if s.Elapsed == "" {
		s.spinner.Draw(v)
		return
	}
	// The elapsed time is pinned to the right and the label gets what is left, so a
	// long label is what gets truncated and the number stays readable.
	elapsed := text.Width(s.Elapsed)
	if elapsed+2 >= width {
		s.spinner.Draw(v)
		return
	}
	s.spinner.Draw(v.Sub(grid.Rect(0, 0, width-elapsed-1, 1)))
	v.Text(width-elapsed, 0, s.Elapsed, s.Theme.Subtle)
}
