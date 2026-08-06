// Package anim is the arithmetic behind things that move.
//
// It is pure functions of a tick and an eased value that advances one step at a
// time. Nothing here holds a clock, opens anything, or knows what a cell is: the
// caller decides when time passes, and what these numbers are then used for —
// brightening a colour, growing a pane — is the caller's.
//
// # Why ticks rather than durations
//
// A transition measured in wall-clock time has to ask what time it is, and a
// animation that asks what time it is cannot be stepped by a test or paused by its owner
// that parked because nothing was happening. Everything here counts ticks instead,
// and a tick is whatever the caller's scheduler decided one is. That makes an
// animation exactly as deterministic as the thing
// driving it, which is the property the rest of this library is built on.
package anim

import "math"

// EaseOutCubic maps a linear position in [0,1] to an eased one.
//
// Out-cubic rather than anything else because it is the curve that reads as
// "arrived": most of the distance is covered early and the last part settles, which
// is what makes a pane that grows look like it stopped rather than like it was cut
// off.
func EaseOutCubic(t float64) float64 {
	if t <= 0 {
		return 0
	}
	if t >= 1 {
		return 1
	}
	u := 1 - t
	return 1 - u*u*u
}

// The shape of the shimmer band: a gaussian highlight riding on a dim base,
// sweeping left to right and wrapping.
const (
	shimmerBase  = 0.35
	shimmerPeak  = 1.0
	shimmerSigma = 3.0
	// shimmerLead is how far outside the row the band's centre starts and ends, so
	// the highlight fades in from the left edge and out past the right one instead
	// of appearing and vanishing.
	shimmerLead      = 8
	shimmerMinPeriod = 24
)

// Shimmer is the brightness at one column of a row being swept by a moving
// highlight — the "this is still arriving" effect on streamed text.
//
// pos is the column, width the row's width, and the result is in [0.35, 1]. It is
// meant for [grid.View.Fade]: fade the column by one minus this, and the sweep reads
// as light rather than as characters changing. A fade rather than a colour, because
// what the text dissolves into is whatever that column is drawn on.
func Shimmer(tick uint64, pos, width int) float64 {
	if width <= 0 {
		return shimmerBase
	}
	period := max(width+2*shimmerLead, shimmerMinPeriod)
	centre := float64(tick%uint64(period)) - shimmerLead
	d := float64(pos) - centre
	return shimmerBase + (shimmerPeak-shimmerBase)*math.Exp(-(d*d)/(2*shimmerSigma*shimmerSigma))
}

// Wave is the brightness of one row of a running accent, offset per row so a
// column of them reads as a wave travelling down rather than as everything
// pulsing at once. The result is in [0.2, 1].
func Wave(tick uint64, row int) float64 {
	phase := float64(tick)*0.35 - float64(row)*(math.Pi/6)
	return 0.2 + 0.8*(math.Sin(phase)+1)/2
}

// Transition moves a value toward a target over a number of ticks, eased.
//
// The zero Transition is settled at zero and animates nothing, so a caller who
// never starts one pays for nothing. [Transition.Tick] is what advances it, and a
// caller with several can drive them all from the one clock they already have.
type Transition struct {
	from, to float64
	// at is how many ticks have been taken of span. A span of zero is a settled
	// transition, which is also what the zero value is.
	at, span uint64
}

// To retargets the transition, starting from wherever it is now. A span of zero
// arrives immediately, which is how to set a value without animating it.
func (t *Transition) To(target float64, span uint64) {
	t.from = t.Value()
	t.to = target
	t.at = 0
	t.span = span
	if span == 0 {
		t.from = target
	}
}

// Tick advances the transition by one step. It is safe to keep calling after it
// has arrived, so a caller does not have to check before stepping.
func (t *Transition) Tick() {
	if t.at < t.span {
		t.at++
	}
}

// Value is where the transition is now.
func (t *Transition) Value() float64 {
	if t.Done() {
		return t.to
	}
	return t.from + (t.to-t.from)*EaseOutCubic(float64(t.at)/float64(t.span))
}

// Done reports whether the transition has arrived, which is what tells its owner it
// can stop the clock driving it.
func (t *Transition) Done() bool { return t.at >= t.span }
