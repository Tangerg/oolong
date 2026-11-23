package kit

import (
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/text"
)

// Status is a [Spinner] that also says how long it has been at it: the spinner,
// its label saying what is happening, and the elapsed time. The caller advances it
// with Tick, exactly as a Spinner.
type Status struct {
	Spinner
	// Elapsed is shown after the label, receding. Empty shows nothing, which is right
	// until something has taken long enough to be worth reporting.
	Elapsed string
}

func (s *Status) Draw(v grid.View) {
	Split{Start: s.line(), End: text.Of(s.Elapsed, s.Theme.Subtle), Ellipsis: s.Glyphs.Ellipsis}.Draw(v)
}
