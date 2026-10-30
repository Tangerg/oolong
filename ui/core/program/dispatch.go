package program

import (
	"errors"
	"sync"

	"github.com/Tangerg/oolong/ui/internal/fifo"
)

var (
	// ErrQueueFull refuses an ordinary action without retaining its callback.
	ErrQueueFull = errors.New("program: action queue is full")
	// ErrCapacity refuses a resource before starting its worker or clock.
	ErrCapacity = errors.New("program: asynchronous resource capacity exhausted")
)

// Dispatcher is the concurrency-safe admission edge of one UI owner. Post never
// waits; callers handle rejection before describing work as accepted.
type Dispatcher struct{ tasks *taskQueue }

const (
	maxTasksPerTurn         = 64
	defaultActionCapacity   = 256
	defaultResourceCapacity = 64
)

type queuedTask struct {
	fn   func() bool
	slot *reservation
}

type taskQueue struct {
	mu                         sync.Mutex
	tasks                      fifo.Queue[queuedTask]
	wake                       chan struct{}
	done                       chan struct{}
	refresh, stopped           bool
	ordinary, resources        int
	capacity, resourceCapacity int
}

func newTaskQueue() *taskQueue {
	return &taskQueue{
		wake: make(chan struct{}, 1), done: make(chan struct{}),
		capacity: defaultActionCapacity, resourceCapacity: defaultResourceCapacity,
	}
}

func (q *taskQueue) post(fn func()) error { return q.postWithEffect(fn, true) }

func (q *taskQueue) postWithEffect(fn func(), changes bool) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.stopped {
		return ErrStopped
	}
	if fn == nil {
		q.refresh = true
	} else {
		if q.ordinary >= q.capacity {
			return ErrQueueFull
		}
		q.tasks.Push(queuedTask{fn: func() bool { fn(); return changes }})
		q.ordinary++
	}
	q.signal()
	return nil
}

// A reservation owns one queue position for an accepted asynchronous resource.
// Completion cannot compete with unrelated actions for the space promised at
// admission. Closed queued slots keep their charge until removed from the FIFO.
type reservation struct {
	queue          *taskQueue
	fn             func() bool
	queued, closed bool
}

func (q *taskQueue) reserve() (*reservation, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.stopped {
		return nil, ErrStopped
	}
	if q.resources >= q.resourceCapacity {
		return nil, ErrCapacity
	}
	q.resources++
	return &reservation{queue: q}, nil
}

func (r *reservation) post(fn func() bool) bool {
	q := r.queue
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.stopped || r.closed {
		return false
	}
	r.fn = fn
	if !r.queued {
		r.queued = true
		q.tasks.Push(queuedTask{slot: r})
	}
	q.signal()
	return true
}

func (r *reservation) close() {
	q := r.queue
	q.mu.Lock()
	defer q.mu.Unlock()
	if r.closed {
		return
	}
	r.closed = true
	r.fn = nil
	if !r.queued {
		q.resources--
	}
}

func (q *taskQueue) take() []func() bool {
	q.mu.Lock()
	batch := q.tasks.Take(maxTasksPerTurn)
	tasks := make([]func() bool, 0, len(batch)+1)
	for _, task := range batch {
		if task.slot == nil {
			q.ordinary--
			tasks = append(tasks, task.fn)
			continue
		}
		slot := task.slot
		slot.queued = false
		if slot.closed {
			q.resources--
			continue
		}
		fn := slot.fn
		slot.fn = nil
		tasks = append(tasks, fn)
	}
	if q.refresh && len(tasks) < maxTasksPerTurn {
		tasks = append(tasks, nil)
		q.refresh = false
	}
	more := q.tasks.Len() > 0 || q.refresh
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

func (q *taskQueue) stop() {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.stopped {
		return
	}
	q.stopped = true
	for _, task := range q.tasks.Take(q.tasks.Len()) {
		if task.slot != nil {
			task.slot.queued = false
			task.slot.fn = nil
			if task.slot.closed {
				q.resources--
			}
		}
	}
	q.ordinary = 0
	q.refresh = false
	close(q.done)
}

// Post accepts fn in FIFO order, or returns ErrQueueFull/ErrStopped without
// retaining it. A nil callback requests a coalesced frame and consumes no slot.
func (d Dispatcher) Post(fn func()) error {
	if d.tasks == nil {
		return ErrStopped
	}
	return d.tasks.post(fn)
}

// Done closes when the owner has stopped accepting work; it is a broadcast, not
// a competing consumer of action or frame notifications.
func (d Dispatcher) Done() <-chan struct{} {
	if d.tasks == nil {
		return closedSignal
	}
	return d.tasks.done
}

var closedSignal = func() <-chan struct{} { done := make(chan struct{}); close(done); return done }()
