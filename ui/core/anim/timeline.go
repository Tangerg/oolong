package anim

import "slices"

type Keyframe struct {
	At    uint64
	Value float64
}

// Timeline owns sequence progress so callers do not maintain a parallel step
// index. It holds its endpoints; the zero value holds zero.
type Timeline struct {
	// frames are owned and strictly ordered so interpolation cannot observe a
	// different sequence from the one Span and Done describe.
	frames []Keyframe
	// Nil Ease is linear: each keyframe already specifies the duration, and
	// easing every interval separately would introduce a stop at each boundary.
	Ease func(t float64) float64
	Loop bool

	tick uint64
}

func NewTimeline(frames ...Keyframe) *Timeline {
	t := &Timeline{}
	t.SetFrames(frames)
	return t
}

// SetFrames replaces the sequence and resets the timeline to its beginning. The
// frames are copied; the caller may reuse or change its input after the call. Ticks
// must be strictly increasing. A duplicate or decreasing tick is a programmer error
// and panics here rather than producing an unsigned interpolation span later.
func (t *Timeline) SetFrames(frames []Keyframe) {
	if t == nil {
		return
	}
	for i := 1; i < len(frames); i++ {
		if frames[i].At <= frames[i-1].At {
			panic("anim: timeline frames are not strictly increasing")
		}
	}
	t.frames = slices.Clone(frames)
	t.tick = 0
}

// Frames returns an owned snapshot so edits cannot alter interpolation.
func (t *Timeline) Frames() []Keyframe {
	if t == nil {
		return nil
	}
	return slices.Clone(t.frames)
}

// Tick advances the timeline by one step. It is safe to keep calling after the end,
// which is what lets a caller drive several from one clock without checking each.
func (t *Timeline) Tick() {
	if t.Loop {
		span := t.Span()
		switch span {
		case 0:
			// The zero timeline and a frame at tick zero are one-state cycles.
			t.tick = 0
		case ^uint64(0):
			// The cycle has every uint64 tick in it. Natural overflow is exactly
			// its wrap; span+1 would itself overflow and make the modulus zero.
			t.tick++
		default:
			t.tick = (t.tick + 1) % (span + 1)
		}
		return
	}
	if !t.Done() {
		t.tick++
	}
}

func (t *Timeline) Reset() { t.tick = 0 }

func (t *Timeline) At() uint64 { return t.tick }

func (t *Timeline) Span() uint64 {
	if len(t.frames) == 0 {
		return 0
	}
	return t.frames[len(t.frames)-1].At
}

// Done remains false while looping, because no terminal state will be reached.
func (t *Timeline) Done() bool { return !t.Loop && t.tick >= t.Span() }

// Value holds the first and last keyframes outside their interval so a late
// draw still receives the endpoint.
func (t *Timeline) Value() float64 {
	if len(t.frames) == 0 {
		return 0
	}
	first := t.frames[0]
	if t.tick <= first.At {
		return first.Value
	}
	for i := 1; i < len(t.frames); i++ {
		to := t.frames[i]
		if t.tick > to.At {
			continue
		}
		from := t.frames[i-1]
		span := to.At - from.At
		at := float64(t.tick-from.At) / float64(span)
		return from.Value + (to.Value-from.Value)*t.shape(at)
	}
	return t.frames[len(t.frames)-1].Value
}

func (t *Timeline) shape(at float64) float64 {
	if t.Ease == nil {
		return at
	}
	return t.Ease(at)
}
