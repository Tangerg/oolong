package base

import "github.com/Tangerg/oolong/ui/internal/counter"

func nextRevision(revision uint64) uint64 {
	next, ok := counter.Next(revision)
	if !ok {
		panic("base: revision exhausted")
	}
	return next
}

// identitySequence hands out monotonically increasing nonzero identities. It is an
// owner's durable namespace, not a recyclable pool: exhaustion is reported before a
// value can wrap and name an object that may still exist outside the component.
type identitySequence struct {
	last uint64
}

func (s *identitySequence) next() (uint64, bool) {
	next, ok := counter.Next(s.last)
	if !ok {
		return 0, false
	}
	s.last = next
	return next, true
}
