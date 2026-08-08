package term

import "sync/atomic"

// atomicSequence reserves monotonically increasing nonzero numbers up to a caller's
// domain limit. It never wraps: once the limit has been handed out, later callers
// are refused rather than being given an identity that may still name old state.
type atomicSequence struct {
	last atomic.Uint64
}

func (s *atomicSequence) next(limit uint64) (uint64, bool) {
	for {
		last := s.last.Load()
		if last >= limit {
			return 0, false
		}
		if s.last.CompareAndSwap(last, last+1) {
			return last + 1, true
		}
	}
}

func (s *atomicSequence) current() uint64 { return s.last.Load() }
