// Package anim advances in caller-owned ticks so animations can pause and tests
// can step them without a wall clock.
package anim

import "math"

// EaseOutCubic settles gradually near the endpoint so a growing region appears
// to arrive instead of stopping abruptly.
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

// Transition is settled at zero until retargeted. Only Tick advances it, so
// multiple transitions can share their owner's clock.
type Transition struct {
	from, to float64
	// A zero span represents a settled transition, including the zero value.
	at, span uint64
}

// To retargets the transition, starting from wherever it is now. A span of zero
// arrives immediately. Non-finite targets panic before changing the transition.
func (t *Transition) To(target float64, span uint64) {
	requireFinite(target)
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

func (t *Transition) Value() float64 {
	if t.Done() {
		return t.to
	}
	return interpolate(t.from, t.to, EaseOutCubic(float64(t.at)/float64(t.span)))
}

func (t *Transition) Done() bool { return t.at >= t.span }

// Opposite-sign endpoints can overflow their difference even when every value
// between them is finite. Exact endpoints also avoid cancellation at keyframes.
func interpolate(from, to, weight float64) float64 {
	switch weight {
	case 0:
		return from
	case 1:
		return to
	}
	if from <= 0 && to >= 0 || from >= 0 && to <= 0 {
		return (1-weight)*from + weight*to
	}
	return math.FMA(weight, to-from, from)
}

func requireFinite(value float64) {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		panic("anim: motion requires finite values")
	}
}
