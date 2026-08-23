package headless

import "math"

// identitySequence hands out monotonically increasing nonzero identities. It is an
// owner's durable namespace, not a recyclable pool: exhaustion is reported before a
// value can wrap and name an object that may still exist outside the component.
type identitySequence struct {
	last uint64
}

func (s *identitySequence) next() (uint64, bool) {
	if s.last == math.MaxUint64 {
		return 0, false
	}
	s.last++
	return s.last, true
}
