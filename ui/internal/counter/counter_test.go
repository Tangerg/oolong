package counter_test

import (
	"math"
	"testing"

	"github.com/Tangerg/oolong/ui/internal/counter"
)

type revision uint64

func TestNextAdvancesUntilTheLargestValue(t *testing.T) {
	if got, ok := counter.Next(revision(41)); got != 42 || !ok {
		t.Fatalf("Next(41) = %d, %t; want 42, true", got, ok)
	}
	if _, ok := counter.Next(revision(math.MaxUint64)); ok {
		t.Fatal("the largest revision had a successor")
	}
	if _, ok := counter.Next(uint32(math.MaxUint32)); ok {
		t.Fatal("the largest 32-bit number had a successor")
	}
}
