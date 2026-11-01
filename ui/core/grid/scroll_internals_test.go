package grid

import (
	"slices"
	"testing"
)

func TestShiftDetectionKeepsTheLongestExactOverlapInEachDirection(t *testing.T) {
	const h = 6
	raster := func(mask int) *Surface {
		s := NewSurface(1, h)
		for i := range h {
			if mask&(1<<i) != 0 {
				s.View().Text(0, i, "x", Style{})
			}
		}
		return s
	}
	for previous := range 1 << h {
		a := raster(previous)
		for current := range 1 << h {
			b := raster(current)
			top, end := 0, h
			for top < end && a.cells[top] == b.cells[top] {
				top++
			}
			for end > top && a.cells[end-1] == b.cells[end-1] {
				end--
			}
			var want []verticalShift
			for _, direction := range []int{1, -1} {
				for d := 1; end-top-d >= minShiftOverlap; d++ {
					from, to := a, b
					if direction < 0 {
						from, to = b, a
					}
					if slices.Equal(from.cells[top+d:end], to.cells[top:end-d]) {
						want = append(want, verticalShift{top: top, bottom: end, delta: direction * d})
						break
					}
				}
			}
			if got := detectShifts(a, b); !slices.Equal(got, want) {
				t.Fatalf("%b -> %b: %v, want %v", previous, current, got, want)
			}
		}
	}
}

func BenchmarkShiftWithRepeatedInteriorRows(b *testing.B) {
	const height = 4096
	previous, current := NewSurface(1, height), NewSurface(1, height)
	previous.View().Text(0, 0, "a", Style{})
	previous.View().Text(0, height-1, "b", Style{})
	current.View().Text(0, height-1, "c", Style{})
	for b.Loop() {
		_ = detectShifts(previous, current)
	}
}
