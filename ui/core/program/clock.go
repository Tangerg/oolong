package program

import (
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

// Timer owns one coalesced scheduled callback. Stop revokes a callback that has
// not started; it cannot undo one already running on the UI owner.
// A Timer must not be copied after construction.
type Timer struct {
	stopped atomic.Bool
	once    sync.Once
	done    chan struct{}
	slot    *reservation
}

// Stop revokes callbacks that have not started and releases reserved capacity.
func (t *Timer) Stop() {
	if t == nil {
		return
	}
	t.once.Do(func() { t.stopped.Store(true); close(t.done); t.slot.close() })
}

// After reserves one delivery before starting the clock. A nonpositive delay
// schedules the next owner turn; closing scope revokes that delivery.
func (r *Runtime) After(scope *Scope, delay time.Duration, fn func()) (*Timer, error) {
	return r.schedule(scope, delay, false, fn)
}

// Every coalesces missed ticks into one reserved callback rather than replaying
// a backlog. The interval must be positive and the callback must not block.
func (r *Runtime) Every(scope *Scope, interval time.Duration, fn func()) (*Timer, error) {
	if interval <= 0 {
		return nil, errors.New("program: repeating interval must be positive")
	}
	return r.schedule(scope, interval, true, fn)
}

func (r *Runtime) schedule(scope *Scope, delay time.Duration, repeat bool, fn func()) (*Timer, error) {
	p := r.owner()
	if p == nil {
		return nil, ErrStopped
	}
	if scope == nil || scope.queue != p.tasks {
		return nil, ErrForeignScope
	}
	if !scope.eligible(p) {
		return nil, ErrScopeClosed
	}
	if fn == nil {
		return nil, errors.New("program: timer requires callback")
	}
	slot, err := p.tasks.reserve()
	if err != nil {
		return nil, err
	}
	timer := &Timer{done: make(chan struct{}), slot: slot}
	deliver := func() bool {
		if timer.stopped.Load() || !scope.eligible(p) {
			return false
		}
		if !repeat {
			defer timer.Stop()
		}
		fn()
		return true
	}
	go timer.run(scope, delay, repeat, deliver)
	return timer, nil
}

func (t *Timer) run(scope *Scope, delay time.Duration, repeat bool, deliver func() bool) {
	clock := time.NewTimer(delay)
	defer clock.Stop()
	defer t.Stop()
	for {
		select {
		case <-scope.ctx.Done():
			return
		case <-scope.queue.done:
			return
		case <-t.done:
			return
		case <-clock.C:
			if !t.slot.post(deliver) {
				return
			}
			if repeat {
				clock.Reset(delay)
				continue
			}
			select {
			case <-t.done:
			case <-scope.ctx.Done():
			case <-scope.queue.done:
			}
			return
		}
	}
}
