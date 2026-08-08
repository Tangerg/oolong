// Package present decides when a frame is drawn.
//
// A terminal UI has more reasons to redraw than it has frames worth drawing: a
// streamed token, a spinner tick, a scroll wheel and a resize can all land inside
// one refresh interval. The presenter turns that stream of reasons into a paced
// sequence of draws, and refuses to draw at all while the terminal is still
// swallowing the last frame.
//
// It is state, not machinery: it owns no goroutine, opens nothing, and writes
// nothing. Its driver asks what to do and it answers. That is what makes the
// pacing rules testable without a terminal.
package present

import "time"

// Presenter tracks whether a frame is owed, whether one is still in flight, and
// when the next one may go.
//
// Not safe for concurrent use. It belongs to the driver that draws, and the whole
// point of a single owner is that "is a frame in flight" has one answer.
type Presenter struct {
	// owed is set by every request and cleared by the draw that satisfies them
	// all. Many requests collapsing into one draw is the coalescing.
	owed bool
	// full accumulates across coalesced requests: once someone has asked for a
	// repaint from scratch, no later plain request may downgrade it.
	full bool

	// inFlight is the writer sequence this presenter is waiting to be told has
	// reached the terminal, or zero when nothing is outstanding.
	inFlight uint64

	drawnAt time.Time
	// dueAt is when a throttled request that was turned away becomes allowed.
	dueAt time.Time
}

// Request asks for a frame, now. It never draws: it records that one is owed, and
// cancels any wait a throttled request had put in the way.
func (p *Presenter) Request() {
	p.owed = true
	p.dueAt = time.Time{}
}

// RequestFull asks for a frame drawn from scratch, for when what the terminal is
// showing can no longer be trusted.
func (p *Presenter) RequestFull() {
	// Through Request, so a repaint is never left waiting behind a throttled stream: the
	// terminal's contents are no longer known, and pacing a correction is pointless.
	p.Request()
	p.full = true
}

// RequestBy asks for a frame no sooner than minInterval after the last one, and reports
// whether it may be drawn straight away.
//
// It is for sources that fire faster than a terminal can usefully be redrawn — a token
// stream, a scroll wheel held down. A request too soon still owes a frame: the interval
// decides when it is drawn, not whether. It arms [Presenter.DueAt] so a driver that parks
// knows when to wake, and [Presenter.Present] holds the frame until then.
func (p *Presenter) RequestBy(now time.Time, minInterval time.Duration) bool {
	p.rebaseClock(now)
	p.owed = true
	if now.Sub(p.drawnAt) < minInterval {
		if p.dueAt.IsZero() {
			p.dueAt = p.drawnAt.Add(minInterval)
		}
		return false
	}
	p.dueAt = time.Time{}
	return true
}

// Present draws the owed frame, if one is owed and the terminal is not still busy
// with the last, and reports whether it drew.
//
// draw is given whether this frame must be a repaint from scratch, and returns the
// writer sequence its bytes were queued under — zero when it queued nothing. A
// frame that reached the writer becomes the one being waited for, so the next
// request coalesces instead of piling a second frame behind the first.
//
// A failed draw does not satisfy the request. Its full-repaint requirement and due
// time remain pending so a caller that can recover may try again; a caller for which
// frame construction is fatal can return the error without the presenter recording a
// frame that never existed.
func (p *Presenter) Present(now time.Time, draw func(full bool) (uint64, error)) (bool, error) {
	p.rebaseClock(now)
	if p.inFlight != 0 || !p.owed {
		return false, nil
	}
	if !p.dueAt.IsZero() && now.Before(p.dueAt) {
		return false, nil
	}
	full := p.full
	seq, err := draw(full)
	if err != nil {
		return false, err
	}
	p.owed, p.full = false, false
	if seq != 0 {
		p.inFlight = seq
	}
	p.drawnAt = now
	p.dueAt = time.Time{}
	return true, nil
}

// rebaseClock forgets pacing deadlines from a later clock epoch. Production callers
// normally pass a monotonic time, but Presenter accepts a clock value precisely so a
// driver can own time and a test can control it. A clock that moved backwards cannot
// meaningfully owe a wait measured from the old epoch.
func (p *Presenter) rebaseClock(now time.Time) {
	if !p.drawnAt.IsZero() && now.Before(p.drawnAt) {
		p.drawnAt = time.Time{}
		p.dueAt = time.Time{}
	}
}

// Wrote reports that the writer has finished with everything up to seq. The frame
// being waited for is released once seq reaches it.
func (p *Presenter) Wrote(seq uint64) {
	if p.inFlight != 0 && seq >= p.inFlight {
		p.inFlight = 0
	}
}

// DueAt is when a turned-away throttled request becomes allowed, if one is pending.
//
// A driver that parks until something happens has to know to wake then, or the last
// update of a burst is never drawn.
func (p *Presenter) DueAt() (time.Time, bool) {
	return p.dueAt, !p.dueAt.IsZero()
}
