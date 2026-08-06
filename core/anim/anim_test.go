package anim_test

import (
	"math"
	"testing"

	"github.com/Tangerg/oolong/core/anim"
)

func TestEaseOutCubicIsClampedAndMonotonic(t *testing.T) {
	if got := anim.EaseOutCubic(-1); got != 0 {
		t.Errorf("before the start = %v, want 0", got)
	}
	if got := anim.EaseOutCubic(2); got != 1 {
		t.Errorf("after the end = %v, want 1", got)
	}
	last := 0.0
	for i := range 101 {
		v := anim.EaseOutCubic(float64(i) / 100)
		if v < last {
			t.Fatalf("eased value went backwards at %d: %v after %v", i, v, last)
		}
		last = v
	}
}

func TestEaseOutCubicCoversMostOfTheDistanceEarly(t *testing.T) {
	// That is what makes a pane that grows read as having arrived rather than as
	// having been cut off. Halfway through the time is well past halfway there.
	if got := anim.EaseOutCubic(0.5); got <= 0.8 {
		t.Fatalf("halfway = %v, want most of the distance already covered", got)
	}
}

func TestShimmerStaysInItsRange(t *testing.T) {
	for tick := range uint64(200) {
		for pos := range 40 {
			v := anim.Shimmer(tick, pos, 40)
			// The range the doc comment promises, spelled out rather than read
			// from the constants: a test that took its expectation from the code
			// under test would pass however that code changed.
			if v < 0.35-1e-9 || v > 1+1e-9 {
				t.Fatalf("Shimmer(%d,%d,40) = %v, outside [0.35,1]", tick, pos, v)
			}
		}
	}
}

func TestShimmerSweepsAcrossTheRow(t *testing.T) {
	// The band has to actually move: the brightest column at one tick must not be
	// the brightest column at every tick, or it is a highlight and not a sweep.
	brightest := func(tick uint64) int {
		at, best := 0, -1.0
		for pos := range 40 {
			if v := anim.Shimmer(tick, pos, 40); v > best {
				at, best = pos, v
			}
		}
		return at
	}
	if brightest(10) == brightest(25) {
		t.Fatal("the shimmer band did not move between ticks")
	}
}

func TestShimmerWithNoRoomIsTheBase(t *testing.T) {
	if got := anim.Shimmer(5, 0, 0); got != 0.35 {
		t.Fatalf("= %v, want the dim base rather than a division by zero", got)
	}
}

func TestWaveStaysInItsRangeAndOffsetsPerRow(t *testing.T) {
	same := true
	for tick := range uint64(100) {
		for row := range 10 {
			v := anim.Wave(tick, row)
			if v < 0.2-1e-9 || v > 1+1e-9 {
				t.Fatalf("anim.Wave(%d,%d) = %v, outside [0.2,1]", tick, row, v)
			}
		}
		if math.Abs(anim.Wave(tick, 0)-anim.Wave(tick, 3)) > 1e-9 {
			same = false
		}
	}
	if same {
		t.Fatal("every row had the same brightness, so it pulses rather than waves")
	}
}

func TestTheZeroTransitionIsSettledAtZero(t *testing.T) {
	var tr anim.Transition
	if !tr.Done() {
		t.Fatal("the zero transition is animating something")
	}
	if got := tr.Value(); got != 0 {
		t.Fatalf("= %v, want 0", got)
	}
	tr.Tick() // must not panic or move
	if got := tr.Value(); got != 0 {
		t.Fatalf("after a tick = %v, want 0", got)
	}
}

func TestATransitionArrivesAfterItsSpan(t *testing.T) {
	var tr anim.Transition
	tr.To(10, 4)
	for range 4 {
		if tr.Done() {
			t.Fatal("arrived before its span was up")
		}
		tr.Tick()
	}
	if !tr.Done() {
		t.Fatal("did not arrive after its span")
	}
	if got := tr.Value(); got != 10 {
		t.Fatalf("= %v, want exactly the target", got)
	}
}

func TestATransitionWithNoSpanArrivesAtOnce(t *testing.T) {
	// Which is how a caller sets a value without animating it.
	var tr anim.Transition
	tr.To(7, 0)
	if !tr.Done() || tr.Value() != 7 {
		t.Fatalf("= %v done=%v, want 7 and settled", tr.Value(), tr.Done())
	}
}

func TestRetargetingStartsFromWhereItGotTo(t *testing.T) {
	// Snapping back to the old start would be visible, and is what makes a widget
	// that is retargeted mid-flight jump.
	var tr anim.Transition
	tr.To(10, 10)
	tr.Tick()
	tr.Tick()
	midway := tr.Value()
	if midway <= 0 || midway >= 10 {
		t.Fatalf("midway = %v, want it somewhere between", midway)
	}
	tr.To(0, 10)
	if got := tr.Value(); math.Abs(got-midway) > 1e-9 {
		t.Fatalf("after retargeting = %v, want it to carry on from %v", got, midway)
	}
}

func TestTickingPastTheEndIsSafe(t *testing.T) {
	var tr anim.Transition
	tr.To(3, 2)
	for range 20 {
		tr.Tick()
	}
	if got := tr.Value(); got != 3 {
		t.Fatalf("= %v, want the target held", got)
	}
}

func TestASpringArrivesAndStops(t *testing.T) {
	// The damping decides what arriving looks like. One arrives and stops; below one
	// it goes past and comes back; above one it creeps in. All three have to end in
	// the same place, or a caller's choice of feel would change where things land.
	for _, damping := range []float64{0.4, 1, 2.5} {
		s := anim.Spring{Frequency: 0.4, Damping: damping}
		s.To(10)
		steps := 0
		for ; !s.Done() && steps < 1000; steps++ {
			s.Tick()
		}
		if steps >= 1000 {
			t.Fatalf("a spring damped %v never settled, at %v", damping, s.Value())
		}
		if got := s.Value(); got != 10 {
			t.Errorf("a spring damped %v settled at %v", damping, got)
		}
	}
}

func TestALooseSpringGoesPastAndAStiffOneDoesNot(t *testing.T) {
	loose := anim.Spring{Frequency: 0.5, Damping: 0.3}
	loose.To(1)
	past := false
	for range 100 {
		loose.Tick()
		past = past || loose.Value() > 1
	}
	if !past {
		t.Error("a spring damped below one never went past its target")
	}

	tight := anim.Spring{Frequency: 0.5, Damping: 1}
	tight.To(1)
	for range 100 {
		tight.Tick()
		if tight.Value() > 1.000001 {
			t.Fatalf("a critically damped spring went past its target, to %v", tight.Value())
		}
	}
}

func TestASpringKeepsTheSpeedItHadWhenTheTargetMoves(t *testing.T) {
	// The whole difference between a spring and a transition. A target that moves
	// twice has to read as one movement, which it cannot if the second one starts
	// from a standstill.
	var s anim.Spring
	s.To(10)
	for range 5 {
		s.Tick()
	}
	moving := s.Velocity()
	if moving <= 0 {
		t.Fatal("a spring on its way somewhere is not moving")
	}
	s.To(20)
	if got := s.Velocity(); got != moving {
		t.Fatalf("moving the target changed the speed from %v to %v", moving, got)
	}
}

func TestATimelineHoldsItsEndsAndPassesThroughItsFrames(t *testing.T) {
	line := anim.Timeline{Frames: []anim.Keyframe{
		{At: 0, Value: 0},
		{At: 4, Value: 1},
		{At: 8, Value: 1},
		{At: 12, Value: 0},
	}}
	want := []float64{0, 0.25, 0.5, 0.75, 1, 1, 1, 1, 1, 0.75, 0.5, 0.25, 0}
	for i, w := range want {
		if got := line.Value(); got != w {
			t.Fatalf("at tick %d the value is %v, want %v", i, got, w)
		}
		line.Tick()
	}
	// Past the end it holds the last frame rather than running off it, so a caller
	// one tick late draws the end state instead of nothing.
	line.Tick()
	if got := line.Value(); got != 0 || !line.Done() {
		t.Fatalf("past the end the value is %v and done is %v", got, line.Done())
	}
}

func TestALoopingTimelineIsNeverDone(t *testing.T) {
	line := anim.Timeline{Loop: true, Frames: []anim.Keyframe{{At: 0, Value: 0}, {At: 2, Value: 1}}}
	seen := make([]float64, 0, 8)
	for range 8 {
		seen = append(seen, line.Value())
		line.Tick()
	}
	for i, want := range []float64{0, 0.5, 1, 0, 0.5, 1, 0, 0.5} {
		if seen[i] != want {
			t.Fatalf("the loop went %v", seen)
		}
	}
	if line.Done() {
		t.Error("a timeline that goes round for ever said it had finished")
	}
}
