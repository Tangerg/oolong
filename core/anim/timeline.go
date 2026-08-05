package anim

// Keyframe is a value a [Timeline] passes through, and when.
type Keyframe struct {
	// At is how many ticks into the timeline this value is reached.
	At uint64
	// Value is what it is then.
	Value float64
}

// Timeline is a value that follows a sequence of keyframes: it eases from each to
// the next and holds the last one.
//
// It is the third kind of movement here, and the one the other two cannot express. A
// [Transition] goes from one value to another and a [Spring] chases a target; a
// sequence — appear, hold, fade — is neither, and writing one out of transitions
// means keeping a step number and a counter beside them and remembering to advance
// both.
//
// The zero Timeline holds zero for ever, which is what a caller who never set any
// frames should get.
type Timeline struct {
	// Frames are the values it passes through, in the order their ticks say. They are
	// read as given: frames out of order are the caller's mistake and are left as
	// such, because sorting them would hide it.
	Frames []Keyframe
	// Ease shapes the movement between two frames. Nil is linear, which is what a
	// sequence of steps usually wants — each step already says how long it takes, and
	// easing every one of them separately reads as a stutter.
	Ease func(t float64) float64
	// Loop starts again from the beginning once the last frame is reached, which is
	// what anything idling — a pulse, a sweep — is made of.
	Loop bool

	tick uint64
}

// Tick advances the timeline by one step. It is safe to keep calling after the end,
// which is what lets a caller drive several from one clock without checking each.
func (t *Timeline) Tick() {
	if t.Loop {
		if span := t.Span(); span > 0 {
			t.tick = (t.tick + 1) % (span + 1)
			return
		}
	}
	if !t.Done() {
		t.tick++
	}
}

// Reset takes the timeline back to its beginning.
func (t *Timeline) Reset() { t.tick = 0 }

// At is how many ticks in the timeline is.
func (t *Timeline) At() uint64 { return t.tick }

// Span is how long the whole timeline is, which is when its last frame is reached.
func (t *Timeline) Span() uint64 {
	if len(t.Frames) == 0 {
		return 0
	}
	return t.Frames[len(t.Frames)-1].At
}

// Done reports whether the last frame has been reached. A looping timeline is never
// done, which is the honest answer: nothing is waiting for it.
func (t *Timeline) Done() bool { return !t.Loop && t.tick >= t.Span() }

// Value is where the timeline is now.
//
// Before the first frame it is the first frame's value, and after the last it is the
// last one's — a timeline holds its ends rather than running off them, so a caller
// that draws one tick late draws the end state instead of nothing.
func (t *Timeline) Value() float64 {
	if len(t.Frames) == 0 {
		return 0
	}
	first := t.Frames[0]
	if t.tick <= first.At {
		return first.Value
	}
	for i := 1; i < len(t.Frames); i++ {
		to := t.Frames[i]
		if t.tick > to.At {
			continue
		}
		from := t.Frames[i-1]
		span := to.At - from.At
		if span == 0 {
			return to.Value
		}
		at := float64(t.tick-from.At) / float64(span)
		return from.Value + (to.Value-from.Value)*t.shape(at)
	}
	return t.Frames[len(t.Frames)-1].Value
}

// shape is the easing, which is linear when none was given.
func (t *Timeline) shape(at float64) float64 {
	if t.Ease == nil {
		return at
	}
	return t.Ease(at)
}
