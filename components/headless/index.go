package headless

// moveIndex applies a signed movement to one position in [0, size). It either
// saturates at the ends or wraps, without adding values that can overflow before the
// range policy is applied. An empty range has no index and returns -1.
func moveIndex(at, by, size int, wrap bool) int {
	if size <= 0 {
		return -1
	}
	at = min(max(at, 0), size-1)
	if !wrap {
		if by >= 0 {
			if by >= size-1-at {
				return size - 1
			}
			return at + by
		}
		if by <= -at {
			return 0
		}
		return at + by
	}

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
