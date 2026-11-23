package base

import "github.com/Tangerg/oolong/ui/core/layout"

// moveIndex applies a signed movement to one position in [0, size). It either
// saturates at the ends or wraps, without adding values that can overflow before the
// range policy is applied. An empty range has no index and returns -1.
func moveIndex(at, by, size int, wrap bool) int {
	if size <= 0 {
		return -1
	}
	if !wrap {
		return saturatingMove(at, by, size-1)
	}
	at = min(max(at, 0), size-1)
	by %= size
	if by >= 0 {
		if room := size - at; by >= room {
			return by - room
		}
		return at + by
	}
	back := -by // by is greater than -size, so negating it cannot overflow.
	if back > at {
		return size - (back - at)
	}
	return at - back
}

// saturatingMove applies a signed movement inside [0, limit] without letting
// either direction wrap before it is clamped.
func saturatingMove(at, by, limit int) int {
	limit = max(limit, 0)
	at = min(max(at, 0), limit)
	if by >= 0 {
		return min(layout.Sum(at, by), limit)
	}
	if by <= -at {
		return 0
	}
	return at + by
}
