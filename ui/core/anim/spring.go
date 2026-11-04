package anim

import "math"

// Default frequency is in radians per tick; critical damping avoids overshoot
// unless the caller explicitly requests it.
const (
	defaultFrequency = 0.35
	defaultDamping   = 1.0

	// A spring approaches its target asymptotically, so position and velocity
	// tolerances determine when its owner can stop the clock.
	settledDistance = 0.001
	settledSpeed    = 0.001
)

// Spring preserves velocity when its target changes so successive changes produce
// continuous motion. It advances in ticks for deterministic stepping. The zero
// value is settled at zero; new targets use non-overshooting defaults.
type Spring struct {
	// Frequency is how fast it moves, in radians per tick: higher is stiffer. A
	// non-positive finite value uses the default. Tick rejects non-finite parameters.
	Frequency float64
	// Damping is how much it overshoots. One arrives and stops; below one it goes
	// past and comes back, and the further below, the more it bounces; above one it
	// creeps in slowly. A non-positive finite value uses the default, which is one.
	Damping float64

	target   float64
	position float64
	velocity float64
}

// To preserves velocity when retargeting. Non-finite targets panic before the
// motion changes.
func (s *Spring) To(target float64) {
	requireFinite(target)
	s.target = target
	s.settle()
}

// Set discards velocity so initialization and resizing do not animate.
// Non-finite values panic before the motion changes.
func (s *Spring) Set(value float64) {
	requireFinite(value)
	s.target, s.position, s.velocity = value, value, 0
}

// Value and Velocity can seed another animation without interrupting motion.
func (s *Spring) Value() float64 { return s.position }

func (s *Spring) Velocity() float64 { return s.velocity }

// Done uses position and velocity tolerances because an exact spring never
// reaches its target in finite time.
func (s *Spring) Done() bool {
	return s.position == s.target && s.velocity == 0
}

func (s *Spring) settle() {
	if math.Abs(s.position-s.target) < settledDistance && math.Abs(s.velocity) < settledSpeed {
		s.Set(s.target)
	}
}

// Tick uses the damped-oscillator solution rather than unstable Euler steps.
// Non-finite parameters or an unrepresentable result panic before state changes.
func (s *Spring) Tick() {
	frequency, damping := s.Frequency, s.Damping
	requireFinite(frequency)
	requireFinite(damping)
	if frequency <= 0 {
		frequency = defaultFrequency
	}
	if damping <= 0 {
		damping = defaultDamping
	}
	if s.Done() {
		return
	}
	step := springResponse(frequency, damping)
	position, velocity := step.advance(s.position, s.target, s.velocity)
	requireFinite(position)
	requireFinite(velocity)
	s.position, s.velocity = position, velocity
	s.settle()
}
