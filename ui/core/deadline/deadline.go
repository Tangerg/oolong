// Package deadline adapts absolute deadlines to a reusable timer so parked
// drivers wake for due work even when no external event arrives.
package deadline

import "time"

// Timer belongs to the goroutine that parks on it; concurrent scheduling and
// stopping are unsupported.
type Timer struct{ timer *time.Timer }

// NewTimer returns a stopped timer. Its caller ends it with [Timer.Stop].
func NewTimer() *Timer {
	timer := time.NewTimer(0)
	// Under the Go 1.27 channel-timer contract Stop and Reset settle the channel
	// themselves, so nothing here drains it and no stale wake-up can arrive late.
	timer.Stop()
	return &Timer{timer: timer}
}

// Schedule arms the timer for at, or disarms it when there is no deadline.
//
// The two arguments are what a deadline owner reports, so a driver hands its answer
// straight over: Schedule(owner.DueAt()). A deadline already past arms for zero,
// which wakes the driver on its next turn rather than never.
func (t *Timer) Schedule(at time.Time, waiting bool) {
	if !waiting {
		t.timer.Stop()
		return
	}
	t.timer.Reset(max(time.Until(at), 0))
}

func (t *Timer) Channel() <-chan time.Time { return t.timer.C }

func (t *Timer) Stop() { t.timer.Stop() }
