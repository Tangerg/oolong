package program

import (
	"sync"
	"sync/atomic"
)

// clockLifetime is the shared cancellation edge of one scheduled callback or
// ticker. Stop may be called concurrently and more than once. Publishing cancelled
// before closing done prevents work already selectable at that instant from posting
// one last stale callback.
type clockLifetime struct {
	done      chan struct{}
	cancelled atomic.Bool
	once      sync.Once
}

func newClockLifetime() *clockLifetime { return &clockLifetime{done: make(chan struct{})} }

func (c *clockLifetime) Stop() {
	c.once.Do(func() {
		c.cancelled.Store(true)
		close(c.done)
	})
}

// coalescedTicks carries one tick at a time to the interface goroutine. A tick that
// arrives while the last is still waiting is dropped rather than queued: a clock
// nobody could keep up with would otherwise grow a backlog of work already stale by
// the time it ran.
type coalescedTicks struct {
	lifetime *clockLifetime
	dispatch Dispatcher
	fn       func()
	pending  atomic.Bool
}

// post offers one tick and reports whether the clock is still running.
func (c *coalescedTicks) post() bool {
	if c.lifetime.cancelled.Load() {
		return false
	}
	if c.pending.CompareAndSwap(false, true) {
		c.dispatch.Post(func() {
			defer c.pending.Store(false)
			if c.lifetime.cancelled.Load() {
				return
			}
			c.fn()
		})
	}
	return true
}
