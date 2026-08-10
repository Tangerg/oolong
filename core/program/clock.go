package program

import (
	"sync"
	"sync/atomic"
	"time"
)

// Dispatcher returns the concurrency-safe handle for background work.
func (r *Runtime) Dispatcher() Dispatcher {
	p := r.owner()
	if p == nil {
		return Dispatcher{}
	}
	return Dispatcher{tasks: p.tasks}
}

// Refresh requests a frame without changing component state.
func (r *Runtime) Refresh() {
	if p := r.owner(); p != nil {
		p.tasks.post(nil)
	}
}

// Quit asks the program to stop.
func (r *Runtime) Quit() {
	p := r.owner()
	if p == nil {
		return
	}
	p.quit.Store(true)
	// Wake a parked loop so it can observe the transition. The signal carries no
	// task and coalesces with any wake-up already waiting.
	p.tasks.signal()
}

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

// After schedules fn once on the interface goroutine after d. The returned stop
// function is concurrency-safe and idempotent. Stop prevents work that has not begun;
// when it races with the callback starting, either may win.
func (r *Runtime) After(d time.Duration, fn func()) (stop func()) {
	p := r.owner()
	if p == nil || d <= 0 || fn == nil {
		return func() {}
	}
	lifetime := newClockLifetime()
	dispatch := r.Dispatcher()
	go func() {
		timer := time.NewTimer(d)
		defer timer.Stop()
		select {
		case <-timer.C:
			if lifetime.cancelled.Load() {
				return
			}
			dispatch.Post(func() {
				if !lifetime.cancelled.Load() {
					fn()
				}
			})
		case <-lifetime.done:
		case <-p.tasks.done:
		}
	}()
	return lifetime.Stop
}

// Every schedules coalesced ticks on the interface goroutine.
func (r *Runtime) Every(d time.Duration, fn func()) (stop func()) {
	p := r.owner()
	if p == nil || d <= 0 || fn == nil {
		return func() {}
	}
	lifetime := newClockLifetime()
	dispatch := r.Dispatcher()
	var pending atomic.Bool
	go func() {
		ticker := time.NewTicker(d)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if lifetime.cancelled.Load() {
					return
				}
				if pending.CompareAndSwap(false, true) {
					dispatch.Post(func() {
						defer pending.Store(false)
						if lifetime.cancelled.Load() {
							return
						}
						fn()
					})
				}
			case <-lifetime.done:
				return
			case <-p.tasks.done:
				return
			}
		}
	}()
	return lifetime.Stop
}
