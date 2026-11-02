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
	// non-positive value uses the default.
	Frequency float64
	// Damping is how much it overshoots. One arrives and stops; below one it goes
	// past and comes back, and the further below, the more it bounces; above one it
	// creeps in slowly. A non-positive value uses the default, which is one.
	Damping float64

	target   float64
	position float64
	velocity float64
}

func (s *Spring) To(target float64) {
	s.target = target
	s.settle()
}

// Set discards velocity so initialization and resizing do not animate.
func (s *Spring) Set(value float64) {
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

// Tick uses the exact damped-oscillator solution: approximate integration can
// make a stiff spring gain energy at ordinary tick rates.
func (s *Spring) Tick() {
	frequency, damping := s.Frequency, s.Damping
	if frequency <= 0 {
		frequency = defaultFrequency
	}
	if damping <= 0 {
		damping = defaultDamping
	}
	if s.Done() {
		return
	}

	// The oscillator equation is centered on zero; translate by the target.
	u, v := s.position-s.target, s.velocity
	switch {
	case damping < 1:
		u, v = ring(u, v, frequency, damping)
	case damping == 1:
		u, v = arrive(u, v, frequency)
	default:
		u, v = creep(u, v, frequency, damping)
	}
	s.position, s.velocity = s.target+u, v
	s.settle()
}

func ring(u, v, frequency, damping float64) (float64, float64) {
	alpha := damping * frequency
	beta := frequency * math.Sqrt(1-damping*damping)
	decay := math.Exp(-alpha)
	sin, cos := math.Sincos(beta)

	position := decay * (u*cos + ((v+alpha*u)/beta)*sin)
	velocity := decay * (v*cos - ((frequency*frequency*u+alpha*v)/beta)*sin)
	return position, velocity
}

func arrive(u, v, frequency float64) (float64, float64) {
	decay := math.Exp(-frequency)
	c := v + frequency*u
	return decay * (u + c), decay * (v - frequency*c)
}

// creep is the over-damped solution: two decaying exponentials, and no overshoot at
// any speed.
func creep(u, v, frequency, damping float64) (float64, float64) {
	root := frequency * math.Sqrt(damping*damping-1)
	r1, r2 := -frequency*damping+root, -frequency*damping-root
	c1 := (v - r2*u) / (r1 - r2)
	c2 := u - c1
	e1, e2 := math.Exp(r1), math.Exp(r2)
	return c1*e1 + c2*e2, c1*r1*e1 + c2*r2*e2
}
