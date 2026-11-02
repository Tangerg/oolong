package retention_test

import (
	"math"
	"testing"

	"github.com/Tangerg/oolong/ui/internal/retention"
)

func TestCompactionReleasesPeaksWithoutPenalizingSmallBuffers(t *testing.T) {
	for _, tc := range []struct {
		capacity, live int
		want           bool
	}{
		{capacity: 0, live: 0},
		{capacity: 8, live: 0, want: true},
		{capacity: 16, live: 1},
		{capacity: 36, live: 10},
		{capacity: 37, live: 10, want: true},
		{capacity: math.MaxInt, live: math.MaxInt},
		{capacity: math.MaxInt, live: math.MaxInt / 2},
		{capacity: math.MaxInt, live: math.MaxInt / 4, want: true},
	} {
		if got := retention.ShouldCompact(tc.capacity, tc.live); got != tc.want {
			t.Errorf("capacity=%d live=%d: compact=%t, want %t", tc.capacity, tc.live, got, tc.want)
		}
	}
}
