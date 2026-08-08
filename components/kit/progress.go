package kit

import (
	"math"
	"strconv"

	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/layout"
	"github.com/Tangerg/oolong/core/text"
)

// percentWidth is the room a percentage is given: enough for a hundred of them.
//
// A fixed field rather than the number's own width, so the bar does not shrink by a
// column when the number goes from nine to ten. A bar that moves backwards while
// the work moves forwards is the one thing a progress bar must not do.
const percentWidth = 4

// Progress shows how far along something with a total is.
//
// It is the other half of [Spinner], and the two are told apart by one question: is
// there a total? Work with one gets a bar, which says how much is left; work without
// one gets a spinner, which can only say that something is happening. A bar drawn
// for work whose size is unknown has to invent a number, and the number it invents
// is the one the user plans around.
//
// It holds nothing. The counts are the caller's, because they are already somewhere
// — a byte count, a file count, a test count — and a widget that kept its own copy
// would be a second place for them to be wrong.
type Progress struct {
	// Theme is the look: the bar is the thing the interface is about, its track is
	// structure to skip, and the numbers beside it are for reference.
	Theme Theme
	// Glyphs are what the bar is drawn with. A progress bar is furniture, so one
	// given no glyphs draws nothing — see the package documentation.
	Glyphs Glyphs

	// Done and Total are the work: how much of it is finished, and how much there is
	// altogether. A total of zero draws an empty bar rather than a full one, which is
	// the honest reading of "nothing to do yet".
	Done, Total int
	// Label goes before the bar, truncated to fit. It is what the work is, and it
	// belongs here rather than above because a bar with no name is a bar nobody can
	// act on when two of them are on screen.
	Label string
	// Percent writes the fraction after the bar as a number.
	Percent bool
}

// Fraction is how much of the work is done, from 0 to 1.
func (p Progress) Fraction() float64 {
	if p.Total <= 0 {
		return 0
	}
	return min(max(float64(p.Done)/float64(p.Total), 0), 1)
}

// Finished reports whether the work is over, which is what a caller asks before
// deciding whether the bar is still worth showing.
func (p Progress) Finished() bool { return p.Total > 0 && p.Done >= p.Total }

// Measure is one row, whatever the width.
func (p Progress) Measure(int) int { return 1 }

// Draw writes the label, the bar and the percentage into the first row of v.
func (p Progress) Draw(v grid.View) {
	w, h := v.Size()
	if w <= 0 || h <= 0 {
		return
	}
	valueWidth := 0
	if p.Percent {
		valueWidth = percentWidth
	}
	boxes := layoutMeter(w, text.Width(p.Label), valueWidth)
	if p.Label != "" {
		Label{Text: p.Label, Style: p.Theme.Muted, Ellipsis: p.Glyphs.Ellipsis}.
			Draw(v.Sub(boxes.label))
	}
	if p.Percent {
		percent := strconv.Itoa(int(math.Round(p.Fraction()*100))) + "%"
		Label{Text: percent, Style: p.Theme.Muted, Align: layout.End}.
			Draw(v.Sub(boxes.value))
	}
	if !boxes.track.Empty() {
		p.bar(v.Sub(boxes.track))
	}
}

// bar paints the track and the part of it that is done.
//
// The cell the bar ends in is drawn as a fraction of itself where the glyph set has
// the pieces for it, which is what keeps a bar from moving in jumps of a whole cell
// on a narrow terminal: eight steps a column is the difference between a bar that
// reads as a measurement and one that reads as a row of blocks.
func (p Progress) bar(v grid.View) {
	w, _ := v.Size()
	if w <= 0 || p.Glyphs.BarFull == "" || p.Glyphs.BarEmpty == "" {
		return
	}
	filled := p.Fraction() * float64(w)
	whole := int(filled)
	steps := p.Glyphs.BarSteps
	step := 0
	if len(steps) > 0 {
		step = min(int((filled-float64(whole))*float64(len(steps)+1)), len(steps))
	}
	for x := range w {
		switch {
		case x < whole:
			v.Text(x, 0, p.Glyphs.BarFull, p.Theme.Accent)
		case x == whole && step > 0:
			v.Text(x, 0, steps[step-1], p.Theme.Accent)
		default:
			v.Text(x, 0, p.Glyphs.BarEmpty, p.Theme.Subtle)
		}
	}
}
