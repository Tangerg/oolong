// Package deadline wakes a driver that parks.
//
// Several things in this library decide when they next need attention and report it
// the same way: a pair of a moment and whether there is one at all. A frame turned
// away for arriving too soon becomes due; an ambiguous escape sequence becomes the
// Escape key. The driver in each case is a select that would otherwise sleep until
// something else happened, and the last update of a burst would sit undelivered.
//
// Turning that pair into one reused timer is the same three lines every time, and
// one of them is a correctness decision rather than a spelling: a deadline already
// past arms for zero rather than for a negative duration. Written once, it is checked
// once.
package deadline

import "time"

// Timer is one reusable wake-up.
//
// It is armed and disarmed from a deadline rather than from a duration, because that
// is the shape the things with deadlines already report — see [Timer.Schedule]. A
// Timer is not safe for concurrent use: it belongs to the one goroutine that parks on
// it.
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

// Channel receives when an armed deadline arrives.
func (t *Timer) Channel() <-chan time.Time { return t.timer.C }

// Stop disarms the timer. It is safe to call more than once.
func (t *Timer) Stop() { t.timer.Stop() }
