package headless

import (
	"math"
	"reflect"
)

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

// sameIdentity reports whether two interface values demonstrably name the same
// object.
//
// Interface equality is not an identity primitive: it panics when a caller's
// concrete widget contains a slice, map, or function. Components are open to external
// implementations, so routing and focus must never inherit that hidden restriction.
// Comparable values can answer directly. Maps have stable runtime identity despite
// not being comparable as interface values. Everything else is conservatively a
// replacement; dropping stale focus or pointer capture is safer than handing the
// remainder of an interaction to an object that may merely look alike.
func sameIdentity(a, b any) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	left, right := reflect.ValueOf(a), reflect.ValueOf(b)
	if left.Type() != right.Type() {
		return false
	}
	if left.Comparable() {
		return left.Interface() == right.Interface()
	}
	if left.Kind() == reflect.Map {
		return left.Pointer() == right.Pointer()
	}
	return false
}
