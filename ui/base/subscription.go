package base

import "slices"

// Subscriptions are owner-sequence notifications. A snapshot allows a callback to
// unsubscribe itself without corrupting the current delivery; removal takes effect
// before the next callback is considered.
type (
	subscription[T any] struct{ callback func(T) }
	notice[T any]       struct {
		receiver *subscription[T]
		value    T
	}
	subscriptions[T any] struct {
		entries    []*subscription[T]
		pending    []notice[T]
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

// Capture membership at commit. Waking scenes may execute application code;
// every committed change still belongs to the listeners present before that wake.
func (s *subscriptions[T]) publish(value T, wake func()) {
	for _, entry := range s.entries {
		s.pending = append(s.pending, notice[T]{entry, value})
	}
	if s.publishing {
		wake()
		return
	}
	s.publishing = true
	defer func() { s.publishing = false }()
	defer s.drain()
	wake()
}

func (s *subscriptions[T]) drain() {
	// Only an abnormal exit can leave work here. Run its successors while the
	// interrupted call unwinds, without replaying it or waiting for a later edit.
	defer func() {
		if len(s.pending) != 0 {
			s.drain()
		}
	}()
	for len(s.pending) != 0 {
		next := s.pending[0]
		s.pending[0] = notice[T]{}
		s.pending = s.pending[1:]
		if len(s.pending) == 0 {
			s.pending = nil
		}
		if fn := next.receiver.callback; fn != nil {
			fn(next.value)
		}
	}
}
