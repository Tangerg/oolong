package base

import "slices"

// Subscriptions are owner-sequence notifications. A snapshot allows a callback to
// unsubscribe itself without corrupting the current delivery; removal takes effect
// before the next callback is considered.
type (
	subscription[T any]  struct{ callback func(T) }
	subscriptions[T any] struct {
		entries    []*subscription[T]
		pending    []T
		publishing bool
	}
)

func (s *subscriptions[T]) add(callback func(T)) func() {
	if callback == nil {
		panic("base: nil subscription")
	}
	entry := &subscription[T]{callback: callback}
	s.entries = append(s.entries, entry)
	return func() {
		if entry.callback == nil {
			return
		}
		entry.callback = nil
		if index := slices.Index(s.entries, entry); index >= 0 {
			s.entries = slices.Delete(s.entries, index, index+1)
		}
	}
}

// publish queues reentrant deliveries behind the current completed notification
// batch. Every observer therefore receives committed versions in source order.
func (s *subscriptions[T]) publish(value T) {
	s.pending = append(s.pending, value)
	if s.publishing {
		return
	}
	s.publishing = true
	defer func() { s.publishing = false; clear(s.pending); s.pending = nil }()
	for len(s.pending) > 0 {
		next := s.pending[0]
		var zero T
		s.pending[0] = zero
		s.pending = s.pending[1:]
		for _, entry := range slices.Clone(s.entries) {
			if entry.callback != nil {
				entry.callback(next)
			}
		}
	}
}
