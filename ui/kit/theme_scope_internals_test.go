package kit

import (
	"math"
	"testing"
)

func TestThemeRevisionExhaustionCannotWrapOrChangePalette(t *testing.T) {
	initial := Dark()
	owner := NewThemeOwner(initial)
	owner.revision = math.MaxUint64
	func() {
		defer func() {
			if recover() == nil {
				t.Error("theme revision wrapped")
			}
		}()
		owner.Set(Light())
	}()
	if owner.revision != math.MaxUint64 || owner.palette != initial {
		t.Fatal("exhausted revision partially changed theme")
	}
}
