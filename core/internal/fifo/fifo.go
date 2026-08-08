// Package fifo provides an ownership-safe in-memory first-in, first-out queue for
// core's internal adapters.
package fifo

import "slices"

// Queue owns an ordered collection of values waiting to be taken.
//
// Removing values clears their slots immediately. After a burst recedes, Queue also
// keeps its backing storage proportional to the live suffix instead of retaining the
// largest historical backlog. The zero value is ready.
type Queue[T any] struct {
	items []T
	head  int
}

// Len reports the number of values waiting.
func (q *Queue[T]) Len() int { return len(q.items) - q.head }

// Push adds value at the end.
func (q *Queue[T]) Push(value T) { q.items = append(q.items, value) }

// Pop removes and returns the oldest value.
func (q *Queue[T]) Pop() (T, bool) {
	if q.Len() == 0 {
		var zero T
		return zero, false
	}
	value := q.items[q.head]
	q.release(1)
	return value, true
}

// Take removes and returns at most limit oldest values in their original order.
// The returned slice is owned by the caller and never aliases Queue's storage.
func (q *Queue[T]) Take(limit int) []T {
	n := min(max(limit, 0), q.Len())
	if n == 0 {
		return nil
	}
	values := slices.Clone(q.items[q.head : q.head+n])
	q.release(n)
	return values
}

// Clear removes every value and releases all storage.
func (q *Queue[T]) Clear() {
	clear(q.items)
	q.items = nil
	q.head = 0
}

func (q *Queue[T]) release(n int) {
	clear(q.items[q.head : q.head+n])
	q.head += n
	live := q.Len()
	if live == 0 {
		q.items = nil
		q.head = 0
		return
	}
	if q.head < len(q.items)/2 {
		return
	}
	if cap(q.items) > 2*live+64 {
		items := make([]T, live)
		copy(items, q.items[q.head:])
		q.items = items
		q.head = 0
		return
	}
	copy(q.items, q.items[q.head:])
	clear(q.items[live:])
	q.items = q.items[:live]
	q.head = 0
}
