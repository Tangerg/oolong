package input_test

import (
	"strconv"
	"testing"
	"time"

	"github.com/Tangerg/oolong/ui/core/input"
)

func TestWheelBatchingChangesStartANewGesture(t *testing.T) {
	for _, test := range []struct {
		name   string
		before input.Wheel
		after  input.Wheel
		want   int
	}{
		{"single report to batched", input.Wheel{Reports: 1, Rows: 3, Trackpad: 15}, input.Wheel{Reports: 3, Rows: 3, Trackpad: 15}, 1},
		{"batched to single report", input.Wheel{Reports: 3, Rows: 3, Trackpad: 15}, input.Wheel{Reports: 1, Rows: 3, Trackpad: 15}, 3},
		{"single report to default batching", input.Wheel{Reports: 1, Rows: 3, Trackpad: 15}, input.Wheel{Rows: 3, Trackpad: 15}, 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			var accumulator input.WheelAccumulator
			accumulator.SetWheel(test.before)
			at := time.Unix(1, 0)
			for report := range 4 {
				accumulator.Accumulate(at.Add(time.Duration(report)*time.Millisecond), 1)
			}
			accumulator.SetWheel(test.after)
			if got := accumulator.Accumulate(at.Add(4*time.Millisecond), 1); got != test.want {
				t.Fatalf("first report under new batching moved %d rows, want %d", got, test.want)
			}
		})
	}
}

func TestWheelCalibrationKeepsFractionalRowsAndCurrentBatchingEvidence(t *testing.T) {
	t.Run("fractional rows", func(t *testing.T) {
		var accumulator input.WheelAccumulator
		accumulator.SetWheel(input.Wheel{Reports: 2, Rows: 1})
		if got := accumulator.Accumulate(time.Time{}, 1); got != 0 {
			t.Fatalf("partial row moved %d rows", got)
		}
		accumulator.SetWheel(input.Wheel{Reports: 1, Rows: 1})
		if got := accumulator.Accumulate(time.Time{}, 1); got != 1 {
			t.Fatalf("new profile moved %d rows, want 1", got)
		}
		accumulator.SetWheel(input.Wheel{Reports: 2, Rows: 1})
		if got := accumulator.Accumulate(time.Time{}, 1); got != 1 {
			t.Fatalf("batching change lost a fractional row: got %d, want 1", got)
		}
	})
	for _, reports := range []int{0, 1, 3} {
		t.Run("equivalent batching "+strconv.Itoa(reports), func(t *testing.T) {
			var accumulator input.WheelAccumulator
			profile := input.Wheel{Reports: reports, Rows: 3, Trackpad: 15}
			accumulator.SetWheel(profile)
			at := time.Unix(1, 0)
			for report := range 4 {
				accumulator.Accumulate(at.Add(time.Duration(report)*time.Millisecond), 1)
			}
			if reports == 0 {
				profile.Reports = 3
			}
			profile.Rows = 6
			accumulator.SetWheel(profile)
			want := 2
			if reports == 1 {
				want = 5
			}
			if got := accumulator.Accumulate(at.Add(4*time.Millisecond), 1); got != want {
				t.Fatalf("unchanged batching moved %d rows, want %d", got, want)
			}
		})
	}
}

func TestZeroWheelReportsCannotAdvanceGestureEvidence(t *testing.T) {
	profile := input.Wheel{Reports: 1, Rows: 3, Trackpad: 15}
	at := time.Unix(1, 0)
	t.Run("cannot keep an old gesture alive", func(t *testing.T) {
		var accumulator input.WheelAccumulator
		accumulator.SetWheel(profile)
		for report := range 4 {
			accumulator.Accumulate(at.Add(time.Duration(report)*time.Millisecond), 1)
		}
		for tick := 1; tick <= 4; tick++ {
			if got := accumulator.Accumulate(at.Add(time.Duration(tick)*100*time.Millisecond), 0); got != 0 {
				t.Fatalf("zero reports moved %d rows", got)
			}
		}
		if got := accumulator.Accumulate(at.Add(500*time.Millisecond), 1); got != 3 {
			t.Fatalf("zero reports kept the old swipe alive: moved %d rows, want 3", got)
		}
	})
	t.Run("cannot begin a later gesture", func(t *testing.T) {
		var accumulator input.WheelAccumulator
		accumulator.SetWheel(profile)
		accumulator.Accumulate(at, 0)
		rows := 0
		for report := range 3 {
			rows = accumulator.Accumulate(at.Add(150*time.Millisecond+time.Duration(report)*time.Millisecond), 1)
		}
		if rows != 5 {
			t.Fatalf("zero reports changed the later swipe's origin: moved %d rows, want 5", rows)
		}
	})
}
