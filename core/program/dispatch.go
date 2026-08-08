package program

import (
	"sync"

	"github.com/Tangerg/oolong/core/internal/fifo"
)

// maxTasksPerTurn bounds how long an already queued burst may keep the owner out
// of its event select. The queue itself remains unbounded and lossless; this is a
// scheduling quantum, not a capacity.
const maxTasksPerTurn = 64

// taskQueue is a concurrent FIFO with a coalesced wake-up.
//
// The queue is deliberately not a channel of tasks. A bounded channel can make the
// interface goroutine wait to enqueue work only that same goroutine can consume; an
// unbounded channel has to be built out of a queue anyway. The mutex owns storage and
// the stopped transition, while wake carries no state and therefore needs room for
// only one unread notification.
type taskQueue struct {
	mu      sync.Mutex
	tasks   fifo.Queue[func()]
	wake    chan struct{}
	done    chan struct{}
	refresh bool
	stopped bool
}

func newTaskQueue() *taskQueue {
	return &taskQueue{
		wake: make(chan struct{}, 1),
		done: make(chan struct{}),
	}
}

// post appends one task without waiting for the interface goroutine. A nil task is
// meaningful: applying it asks for a frame without changing state.
func (q *taskQueue) post(fn func()) bool {
	q.mu.Lock()
	if q.stopped {
		q.mu.Unlock()
		return false
	}
	if fn == nil {
		// A refresh carries no state and the loop draws only after the batch. Keeping
		// more than one would consume memory without changing a frame.
		if q.refresh {
			q.mu.Unlock()
			return true
		}
		q.refresh = true
	} else {
		q.tasks.Push(fn)
	}
	q.mu.Unlock()
	q.signal()
	return true
}

// take removes at most one owner turn of work. It wakes the owner again when more
// remains, giving input, writer progress and frame deadlines a chance between
// portions of a burst whether that burst arrived before or during this call.
func (q *taskQueue) take() []func() {
	q.mu.Lock()
	tasks := q.tasks.Take(maxTasksPerTurn)
	if q.tasks.Len() == 0 && q.refresh {
		tasks = append(tasks, nil)
		q.refresh = false
	}
	more := q.tasks.Len() > 0
	q.mu.Unlock()
	if more {
		q.signal()
	}
	return tasks
}

func (q *taskQueue) signal() {
	select {
	case q.wake <- struct{}{}:
	default:
	}
}

// stop prevents new work, drops work that can no longer be shown and ends clocks
// waiting on the dispatcher. It belongs to the interface goroutine and runs once.
func (q *taskQueue) stop() {
	q.mu.Lock()
	if q.stopped {
		q.mu.Unlock()
		return
	}
	q.stopped = true
	q.tasks.Clear()
	q.refresh = false
	close(q.done)
	q.mu.Unlock()
}

// Post runs fn on the interface goroutine and requests a frame afterwards. A nil
// function requests only the frame. Calls never wait, preserve FIFO acceptance
// order, and are dropped after the program stops or on a zero Dispatcher.
//
// Never waiting is the whole point and it is also the whole cost: the queue has no
// upper bound, so a producer posting faster than the interface goroutine can drain
// grows it without limit. Nothing here can fix that, because the only two things a
// queue can do when it is full are block the caller and lose work, and this edge
// exists to do neither. Coalescing therefore belongs to the caller, at the source:
// post the state a burst arrived at rather than one call per item, or keep the state
// somewhere the interface goroutine reads and post nothing but a request for a frame
// — which is what a nil function is for, and why several of them collapse into one.
// [Runtime.Every] is the same discipline applied to a clock.
func (d Dispatcher) Post(fn func()) {
	_ = d.post(fn)
}

// Done is closed when the program can no longer accept or apply work. The zero
// Dispatcher's channel is already closed. A background producer selects on Done to
// stop work that has no remaining owner.
func (d Dispatcher) Done() <-chan struct{} {
	if d.tasks == nil {
		return closedSignal
	}
	return d.tasks.done
}

func (d Dispatcher) post(fn func()) bool {
	return d.tasks != nil && d.tasks.post(fn)
}

var closedSignal = func() <-chan struct{} {
	done := make(chan struct{})
	close(done)
	return done
}()
