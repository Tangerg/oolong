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
