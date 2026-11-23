package layout

import "math/bits"

// Translate moves a signed coordinate by delta, saturating at the integer limits.
// It is distinct from [Sum]: coordinates may legitimately be negative, while Sum
// combines non-negative extents and treats a negative input as no extent.
func Translate(at, delta int) int {
	switch {
	case delta > 0 && at > maxInt-delta:
		return maxInt
	case delta < 0 && at < minInt-delta:
		return minInt
	default:
		return at + delta
	}
}

// Relative projects an absolute coordinate into a space whose origin is origin,
// saturating when the mathematical difference is outside the integer range.
func Relative(at, origin int) int {
	switch {
	case origin > 0 && at < minInt+origin:
		return minInt
	case origin < 0 && at > maxInt+origin:
		return maxInt
	default:
		return at - origin
	}
}

// Sum adds non-negative extents, saturating at the largest int instead of letting
// overflow turn geometry negative. Negative inputs describe no extent and contribute
// zero. It is the one addition rule for both layout internals and composed measurers.
func Sum(extents ...int) int {
	total := 0
	for _, extent := range extents {
		extent = max(extent, 0)
		if extent > maxInt-total {
			return maxInt
		}
		total += extent
	}
	return total
}

// Remaining subtracts non-negative extents from total without underflow. Negative
// inputs describe no room or no use. It is the subtraction counterpart to [Sum] and
// the one rule for asking how much of a measured axis is left.
func Remaining(total int, used ...int) int {
	total = max(total, 0)
	for _, extent := range used {
		extent = max(extent, 0)
		if extent >= total {
			return 0
		}
		total -= extent
	}
	return total
}

// Scale returns total*part/whole, capped to [0, total], without overflowing the
// intermediate product.
//
// It is the one proportional-coordinate operation shared by layout allocation and
// controls that map a bounded value onto an extent. Keeping it here gives both the
// same endpoint, saturation, and architecture-width semantics instead of two subtly
// different overflow workarounds.
func Scale(total, part, whole int) int {
	if total <= 0 || part <= 0 || whole <= 0 {
		return 0
	}
	if part >= whole {
		return total
	}
	hi, lo := bits.Mul64(uint64(total), uint64(part))
	quotient, _ := bits.Div64(hi, lo, uint64(whole))
	if quotient > uint64(maxInt) {
		return maxInt
	}
	return int(quotient)
}
