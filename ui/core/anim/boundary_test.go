package anim_test

import (
	"math"
	"slices"
	"testing"

	"github.com/Tangerg/oolong/ui/core/anim"
)

func TestFiniteTransitionPreservesItsStartingPointAndRange(t *testing.T) {
	var transition anim.Transition
	transition.To(-math.MaxFloat64, 0)
	transition.To(math.MaxFloat64, 2)
	if got := transition.Value(); got != -math.MaxFloat64 {
		t.Fatalf("retargeted start = %g", got)
	}
	transition.Tick()
	want := 0.75 * math.MaxFloat64
	if got := transition.Value(); math.Abs(got/want-1) > 1e-15 {
		t.Fatalf("eased interior = %g, want %g", got, want)
	}
	transition.Tick()
	if !transition.Done() || transition.Value() != math.MaxFloat64 {
		t.Fatal("transition did not hold its exact endpoint")
	}
}

func TestTimelinePreservesFiniteInterpolationAndExactKeyframes(t *testing.T) {
	line := anim.NewTimeline(anim.Keyframe{At: 0, Value: -math.MaxFloat64}, anim.Keyframe{At: 2, Value: math.MaxFloat64})
	line.Tick()
	if got := line.Value(); got != 0 {
		t.Fatalf("linear midpoint = %g, want 0", got)
	}
	line.Tick()
	if got := line.Value(); !line.Done() || got != math.MaxFloat64 {
		t.Fatalf("endpoint = %g, done = %t", got, line.Done())
	}
	line.SetFrames([]anim.Keyframe{{At: 0, Value: 1e16}, {At: 1, Value: 1}, {At: 2, Value: 2}})
	line.Tick()
	if got := line.Value(); got != 1 {
		t.Fatalf("interior keyframe = %g, want exactly 1", got)
	}
}

func TestTransitionRejectsNonFiniteTargetsBeforeMutation(t *testing.T) {
	for _, invalid := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		var transition anim.Transition
		transition.To(10, 4)
		transition.Tick()
		expected := transition
		assertPanics(t, func() { transition.To(invalid, 2) })
		for range 5 {
			if transition.Value() != expected.Value() || transition.Done() != expected.Done() {
				t.Fatal("rejected target changed the transition")
			}
			transition.Tick()
			expected.Tick()
		}
	}
}

func TestTimelineRejectsInvalidFramesBeforeMutation(t *testing.T) {
	for _, test := range []struct {
		name   string
		frames []anim.Keyframe
	}{
		{"first value", []anim.Keyframe{{At: 0, Value: math.NaN()}, {At: 3, Value: 1}}},
		{"middle value", []anim.Keyframe{{At: 0, Value: 1}, {At: 1, Value: math.Inf(-1)}, {At: 3, Value: 2}}},
		{"last value", []anim.Keyframe{{At: 0, Value: 1}, {At: 3, Value: math.Inf(1)}}},
		{"duplicate tick", []anim.Keyframe{{At: 2, Value: 1}, {At: 2, Value: 2}}},
		{"decreasing tick", []anim.Keyframe{{At: 3, Value: 1}, {At: 1, Value: 2}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			line := anim.NewTimeline(anim.Keyframe{At: 1, Value: 2}, anim.Keyframe{At: 4, Value: 8})
			line.Tick()
			line.Tick()
			frames, at, value := line.Frames(), line.At(), line.Value()
			assertPanics(t, func() { line.SetFrames(test.frames) })
			if !slices.Equal(line.Frames(), frames) || line.At() != at || line.Value() != value {
				t.Fatal("rejected frames changed the timeline")
			}
			line.Tick()
			if line.Value() != 6 || line.Done() {
				t.Fatal("rejected frames changed subsequent progress")
			}
		})
	}
}

func TestTimelineRejectsInvalidEasingWithoutChangingProgress(t *testing.T) {
	for _, weight := range []float64{math.NaN(), math.Inf(1), math.Inf(-1), 2} {
		line := anim.NewTimeline(anim.Keyframe{At: 0, Value: -math.MaxFloat64}, anim.Keyframe{At: 2, Value: math.MaxFloat64})
		line.Tick()
		frames := line.Frames()
		line.Ease = func(float64) float64 { return weight }
		assertPanics(t, func() { _ = line.Value() })
		if line.At() != 1 || line.Span() != 2 || line.Done() || !slices.Equal(line.Frames(), frames) {
			t.Fatal("invalid easing changed the timeline")
		}
		line.Ease = nil
		if line.Value() != 0 {
			t.Fatal("invalid easing changed subsequent interpolation")
		}
		line.Tick()
		if line.Value() != math.MaxFloat64 || !line.Done() {
			t.Fatal("invalid easing changed the endpoint")
		}
	}
}

func TestSpringLargeFiniteParametersRemainStable(t *testing.T) {
	for _, test := range []struct {
		name                     string
		frequency, damping, want float64
	}{
		{name: "stiff damped motion", frequency: 1e200, damping: 0.5, want: 1},
		{name: "very strong damping", frequency: 1, damping: 1e200, want: 5e-200},
		{name: "slow decaying root", frequency: 1, damping: 1e8, want: 4.99999985e-8},
	} {
		t.Run(test.name, func(t *testing.T) {
			spring := anim.Spring{Frequency: test.frequency, Damping: test.damping}
			spring.To(1)
			for range 10 {
				spring.Tick()
			}
			if got := spring.Value(); math.IsNaN(got) || math.IsInf(got, 0) || math.Abs(got/test.want-1) > 1e-7 {
				t.Fatalf("position = %g, want approximately %g", got, test.want)
			}
			if speed := spring.Velocity(); math.IsNaN(speed) || math.IsInf(speed, 0) {
				t.Fatalf("non-finite velocity %g", speed)
			}
		})
	}
}

func TestSpringSmallFrequencyPreservesRepresentableMotion(t *testing.T) {
	for _, test := range []struct {
		name                                 string
		frequency, damping, target, position float64
		velocity                             float64
	}{
		{"underdamped", 1e-50, 0.5, 1e100, 0.5, 1},
		{"critical", 1e-50, 1, 1e100, 0.5, 1},
		{"overdamped", 1e-50, 2, 1e100, 0.5, 1},
		{"underflowing squared frequency", 1e-200, 1, 1e300, 5e-101, 1e-100},
		{
			name: "underflowing slow root", frequency: 1e-200, damping: 1e200, target: 1e300,
			position: (1 + math.Exp(-2)) / 4 * 1e-100,
			velocity: -math.Expm1(-2) / 2 * 1e-100,
		},
		{
			name: "extreme damping", frequency: 1e-300, damping: math.MaxFloat64, target: math.MaxFloat64,
			position: 5e-301 * (1 - 1/(2*1e-300*math.MaxFloat64)), velocity: 5e-301,
		},
		{"subnormal slow root", 1e-100, 1e223, 1e300, 5e-24, 5e-24},
		{"overflowing fast root", 1, math.MaxFloat64, math.MaxFloat64, 0.5, 0.5},
		{
			name: "extreme frequency and damping", frequency: math.MaxFloat64, damping: math.MaxFloat64, target: math.MaxFloat64,
			position: -math.Expm1(-0.5) * math.MaxFloat64, velocity: (0.5 * math.Exp(-0.5)) * math.MaxFloat64,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			spring := anim.Spring{Frequency: test.frequency, Damping: test.damping}
			spring.To(test.target)
			spring.Tick()
			assertRelative(t, "position", spring.Value(), test.position)
			assertRelative(t, "velocity", spring.Velocity(), test.velocity)
		})
	}
}

func TestSpringTinyFrequencyRetargetingKeepsExistingVelocity(t *testing.T) {
	for _, damping := range []float64{0.5, 1, 2} {
		spring := anim.Spring{Frequency: 1, Damping: 1}
		spring.To(1)
		spring.Tick()
		position, velocity := spring.Value(), spring.Velocity()
		spring.Frequency, spring.Damping = math.SmallestNonzeroFloat64, damping
		spring.To(1e300)
		if spring.Velocity() != velocity {
			t.Fatal("retargeting discarded velocity")
		}
		spring.Tick()
		assertRelative(t, "position", spring.Value(), position+velocity)
		assertRelative(t, "velocity", spring.Velocity(), velocity)
	}
}

func TestSpringOppositeExtremeEndpointsDoNotOverflowTheirDifference(t *testing.T) {
	for _, direction := range []float64{-1, 1} {
		spring := anim.Spring{Frequency: 1, Damping: 1}
		spring.Set(-direction * math.MaxFloat64)
		spring.To(direction * math.MaxFloat64)
		for tick := range 5 {
			spring.Tick()
			elapsed := float64(tick + 1)
			decay := math.Exp(-elapsed)
			wantPosition := direction * (1 - 2*(1+elapsed)*decay) * math.MaxFloat64
			wantVelocity := direction * (2 * elapsed * decay) * math.MaxFloat64
			assertRelative(t, "position", spring.Value(), wantPosition)
			assertRelative(t, "velocity", spring.Velocity(), wantVelocity)
		}
	}
}

func TestSpringDecayRemainsAvailableToLargeFiniteState(t *testing.T) {
	spring := anim.Spring{Frequency: 1e200, Damping: 1e-197}
	spring.Set(math.MaxFloat64)
	spring.To(0)
	spring.Tick()
	attenuation := spring.Frequency * spring.Damping
	decayedPosition := (math.Exp(-attenuation/2) * math.MaxFloat64) * math.Exp(-attenuation/2)
	wantPosition := math.Cos(spring.Frequency) * decayedPosition
	wantVelocity := -(spring.Frequency * decayedPosition) * math.Sin(spring.Frequency)
	assertRelative(t, "position", spring.Value(), wantPosition)
	assertRelative(t, "velocity", spring.Velocity(), wantVelocity)
}

func TestSpringRejectsUnrepresentableMotionBeforeMutation(t *testing.T) {
	for _, test := range []struct {
		name               string
		frequency, damping float64
	}{
		{"position", math.Pi, math.SmallestNonzeroFloat64},
		{"velocity", 1e200, 1e-200},
	} {
		t.Run(test.name, func(t *testing.T) {
			spring := anim.Spring{Frequency: test.frequency, Damping: test.damping}
			spring.To(math.MaxFloat64)
			assertPanicsWithoutSpringMutation(t, &spring, spring.Tick)
			spring.Frequency, spring.Damping = 1, 1
			spring.Tick()
			assertRelative(t, "position after rejected tick", spring.Value(), (1-2/math.E)*math.MaxFloat64)
		})
	}
}

func TestSpringRejectsNonFiniteInputsBeforeMutation(t *testing.T) {
	spring := anim.Spring{Frequency: 1, Damping: 1}
	spring.To(math.MaxFloat64)
	for _, invalid := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		assertPanicsWithoutSpringMutation(t, &spring, func() { spring.To(invalid) })
		assertPanicsWithoutSpringMutation(t, &spring, func() { spring.Set(invalid) })
		spring.Frequency = invalid
		assertPanicsWithoutSpringMutation(t, &spring, spring.Tick)
		spring.Frequency, spring.Damping = 1, invalid
		assertPanicsWithoutSpringMutation(t, &spring, spring.Tick)
	}
	spring.Frequency, spring.Damping = 1, 1
	spring.Tick()
	assertRelative(t, "position after rejected tick", spring.Value(), (1-2/math.E)*math.MaxFloat64)
}

func assertRelative(t *testing.T, quantity string, got, want float64) {
	t.Helper()
	if math.IsNaN(got) || math.IsInf(got, 0) || math.Abs(got/want-1) > 2e-14 {
		t.Fatalf("%s = %g, want approximately %g", quantity, got, want)
	}
}

func assertPanicsWithoutSpringMutation(t *testing.T, spring *anim.Spring, action func()) {
	t.Helper()
	position, velocity, done := spring.Value(), spring.Velocity(), spring.Done()
	assertPanics(t, action)
	if spring.Value() != position || spring.Velocity() != velocity || spring.Done() != done {
		t.Fatal("invalid motion changed the spring")
	}
}

func assertPanics(t *testing.T, action func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Fatal("invalid animation input did not panic")
		}
	}()
	action()
}
