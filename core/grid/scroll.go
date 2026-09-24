package grid

import (
	"image"
	"slices"
)

// minShiftOverlap is how many rows a shift must carry over for it to be worth
// asking the terminal to move them. Below that there is nothing to save.
const minShiftOverlap = 2

// verticalShift is a whole-width move of the rows in [top, bottom). A positive
// delta moves them up, a negative one down, and the rows the move exposes are
// repainted afterwards.
type verticalShift struct {
	top, bottom int
	delta       int
}

// detectShifts finds the row shifts that would turn prev into next: at most one
// upward and one downward candidate.
//
// The region considered is the band of rows that actually changed, so everything
// outside it is known to be identical and can be left out of the scrolling
// region. Within a direction the smallest shift wins, because it is the one that
// carries the most rows over and exposes the fewest.
func detectShifts(prev, next *Surface) []verticalShift {
	if !diffable(prev, next) || prev.h <= minShiftOverlap {
		return nil
	}

	top := 0
	for top < next.h && rowEqual(prev, top, next, top) {
		top++
	}
	if top == next.h {
		return nil // nothing changed at all
	}
	bottom := next.h
	for bottom > top && rowEqual(prev, bottom-1, next, bottom-1) {
		bottom--
	}
	height := bottom - top
	if height <= minShiftOverlap {
		return nil
	}

	var shifts []verticalShift
	for delta := 1; height-delta >= minShiftOverlap; delta++ {
		if rowsEqual(prev, top+delta, next, top, height-delta) {
			shifts = append(shifts, verticalShift{top: top, bottom: bottom, delta: delta})
			break
		}
	}
	for delta := 1; height-delta >= minShiftOverlap; delta++ {
		if rowsEqual(prev, top, next, top+delta, height-delta) {
			shifts = append(shifts, verticalShift{top: top, bottom: bottom, delta: -delta})
			break
		}
	}
	return shifts
}

func (v verticalShift) exposed() (y0, y1 int) {
	if v.delta > 0 {
		return v.bottom - v.delta, v.bottom
	}
	return v.top, v.top - v.delta
}

// diffCostFloor is a guaranteed lower bound on what the plain cell diff between
// prev and next will cost in bytes.
//
// It counts positioning and printable bytes exactly, and deliberately leaves out
// style transitions: those can only add to the real stream, so a shift that beats
// this floor beats the diff itself. A floor rather than the diff means the diff is
// never built just to be thrown away.
func diffCostFloor(prev, next *Surface) int {
	if !diffable(prev, next) {
		return 0
	}
	dirty := changedAgainst(prev, next)
	cost := 0
	var at image.Point
	known := false
	for y := range next.h {
		for x := 0; x < next.w; {
			i := y*next.w + x
			unit := paintUnitAt(next.cells, i, next.w-x)
			if !dirty(i, unit.width) {
				x += unit.width
				continue
			}
			if cost == 0 {
				// The frame's leading and trailing style resets.
				cost = 2 * len(sgrReset)
			}
			if !known || at.X != x || at.Y != y {
				cost += cupCost(x, y)
			}
			cost += unit.byteCost()
			at, known = image.Pt(x+unit.width, y), true
			x += unit.width
		}
	}
	return cost
}

func cupCost(x, y int) int {
	return len("\x1b[;H") + digits(y+1) + digits(x+1)
}

func digits(n int) int {
	d := 1
	for n >= 10 {
		n /= 10
		d++
	}
	return d
}

func diffable(a, b *Surface) bool {
	return a != nil && b != nil &&
		a.w == b.w && a.h == b.h && a.w > 0 && a.h > 0 &&
		len(a.cells) == a.w*a.h && len(b.cells) == b.w*b.h
}

func rowEqual(a *Surface, ay int, b *Surface, by int) bool {
	return slices.Equal(a.row(ay), b.row(by))
}

// rowsEqual reports whether n rows of a starting at aTop match n rows of b
// starting at bTop.
func rowsEqual(a *Surface, aTop int, b *Surface, bTop, n int) bool {
	for i := range n {
		if !rowEqual(a, aTop+i, b, bTop+i) {
			return false
		}
	}
	return true
}
