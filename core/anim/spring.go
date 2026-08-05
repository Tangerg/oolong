package anim

import "math"

// The numbers a spring falls back on when it was given none.
//
// A frequency in radians per tick, which at the frame rate a program draws at
// settles in about a second, and the damping that arrives without going past —
// because overshooting is a decision and not a default.
const (
	defaultFrequency = 0.35
	defaultDamping   = 1.0

	// settled is how close to the target, and how slow, a spring has to be before it
	// is called arrived. A spring never reaches its target exactly — the maths says
	// so — so something has to say when it is close enough to stop the clock.
	settledDistance = 0.001
	settledSpeed    = 0.001
)

// Spring moves a value toward a target the way a weight on a spring does: it
// gathers speed, arrives, and — if it is allowed to — goes a little past and comes
// back.
//
// It is the other kind of movement from [Transition], and the difference is what
// happens when the target changes half way. A transition restarts, easing from
// wherever it was, and the change reads as a stop and a fresh start. A spring keeps
// the speed it already had, so a target that moves twice reads as one continuous
// movement — which is what makes it the right thing for anything the user is
// dragging, resizing or scrolling.
//
// Like everything here it is stepped in ticks rather than in seconds, so a test
// steps it and gets the same numbers every time.
//
// The zero Spring is settled at zero and moves nothing. Given a target it uses the
// defaults above, which arrive without overshooting.
type Spring struct {
	// Frequency is how fast it moves, in radians per tick: higher is stiffer. Zero
	// uses the default.
	Frequency float64
	// Damping is how much it overshoots. One arrives and stops; below one it goes
	// past and comes back, and the further below, the more it bounces; above one it
	// creeps in slowly. Zero uses the default, which is one.
	Damping float64

	target   float64
	position float64
	velocity float64
}

// To sets what the spring is moving toward, keeping the speed it already has.
func (s *Spring) To(target float64) { s.target = target }

// Set puts the value where it is asked and stops it there, which is how to place a
// spring without animating it — the first frame, or a resize nobody should watch.
func (s *Spring) Set(value float64) {
	s.target, s.position, s.velocity = value, value, 0
}

// Value is where the spring is now, and Velocity how fast it is moving. The speed is
// worth having: something being flung reads as the same movement continuing only if
// whatever takes over starts with the speed it had.
func (s *Spring) Value() float64 { return s.position }

// Velocity is how fast the value is changing, per tick.
func (s *Spring) Velocity() float64 { return s.velocity }

// Settled reports whether the spring has arrived: near enough its target and slow
// enough that nothing further would be visible. It is what tells a loop it can stop
// the clock driving this.
func (s *Spring) Settled() bool {
	return math.Abs(s.position-s.target) < settledDistance && math.Abs(s.velocity) < settledSpeed
}

// Tick advances the spring by one step.
//
// The step is the exact solution of the equation rather than a small step of it,
// which matters at a terminal's frame rate: stepping a stiff spring approximately,
// sixty times a second, is how a spring turns into an oscillation that grows instead
// of one that dies away. There are three solutions and which one applies is decided
// by the damping — under one it rings, at one it just arrives, above one it creeps —
// so all three are here rather than the one that is usually enough.
func (s *Spring) Tick() {
	frequency, damping := s.Frequency, s.Damping
	if frequency <= 0 {
		frequency = defaultFrequency
	}
	if damping <= 0 {
		damping = defaultDamping
	}
	if s.Settled() {
		s.position, s.velocity = s.target, 0
		return
	}

	// Everything is worked out on the distance still to go, which is what the
	// equation is written in: a spring is a value pulled toward zero, and the target
	// is where zero is.
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
	if s.Settled() {
		// Put exactly there. The maths never quite arrives, and a value that stays at
		// 9.9996 for ever is one that whoever rounds it draws as nine.
		s.position, s.velocity = s.target, 0
	}
}

// ring is the under-damped solution: it goes past and comes back, fading.
func ring(u, v, frequency, damping float64) (float64, float64) {
	alpha := damping * frequency
	beta := frequency * math.Sqrt(1-damping*damping)
	decay := math.Exp(-alpha)
	sin, cos := math.Sincos(beta)

	position := decay * (u*cos + ((v+alpha*u)/beta)*sin)
	velocity := decay * (v*cos - ((frequency*frequency*u+alpha*v)/beta)*sin)
	return position, velocity
}

// arrive is the critically damped solution: the fastest approach with no overshoot.
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
