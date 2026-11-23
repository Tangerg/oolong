// Package weakset owns registrations that do not keep their members alive, so a
// long-lived owner can notify short-lived consumers without retaining them.
package weakset

import (
	"slices"
	"weak"

	"github.com/Tangerg/oolong/ui/internal/retention"
)

// Set is a registry of weakly held members in registration order. The zero value
// is empty. A Set belongs to its owner's goroutine.
type Set[T any] struct{ members []weak.Pointer[T] }

// Add registers member. A full registry first drops collected members, so its
// storage follows the live set rather than every member it ever held.
func (s *Set[T]) Add(member *T) {
	if len(s.members) == cap(s.members) {
		s.Prune(nil)
	}
	s.members = append(s.members, weak.Make(member))
}

// Prune drops collected members and, when keep is not nil, the live members keep
// refuses, releasing storage a past peak left behind.
func (s *Set[T]) Prune(keep func(*T) bool) {
	live := s.members[:0]
	for _, reference := range s.members {
		if member := reference.Value(); member != nil && (keep == nil || keep(member)) {
			live = append(live, reference)
		}
	}
	clear(s.members[len(live):])
	switch {
	case len(live) == 0:
		s.members = nil
	case retention.ShouldCompact(cap(live), len(live)):
		s.members = slices.Clone(live)
	default:
		s.members = live
	}
}

// Live returns the members still alive, in registration order. The slice is the
// caller's, so a callback that registers or prunes cannot disturb the iteration.
func (s *Set[T]) Live() []*T {
	var live []*T
	for _, reference := range s.members {
		if member := reference.Value(); member != nil {
			live = append(live, member)
		}
	}
	return live
}
