package present_test

import (
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/oolong/core/present"
)

var epoch = time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)

// drew records how a draw was asked for and what it queued.
type drew struct {
	calls int
	full  bool
	seq   uint64
}

func (d *drew) draw(full bool) (uint64, error) {
	d.calls++
	d.full = full
	return d.seq, nil
}

func presented(t *testing.T, p *present.Presenter, now time.Time, draw func(bool) (uint64, error)) bool {
	t.Helper()
	drew, err := p.Present(now, draw)
	if err != nil {
		t.Fatalf("Present: %v", err)
	}
	return drew
}

func TestNothingIsDrawnUntilSomethingAsks(t *testing.T) {
	var p present.Presenter
	var d drew
	if presented(t, &p, epoch, d.draw) {
		t.Fatal("drew a frame nobody asked for")
	}
	// And still nothing once something is drawn for the first time.
	if presented(t, &p, epoch, d.draw) {
		t.Fatal("drew a frame nobody asked for")
	}
}

func TestManyRequestsCollapseIntoOneFrame(t *testing.T) {
	var p present.Presenter
	d := drew{seq: 1}
	for range 10 {
		p.Request()
	}
	if !presented(t, &p, epoch, d.draw) || d.calls != 1 {
		t.Fatalf("drew %d times, want exactly one frame for ten requests", d.calls)
	}
	// The frame is in flight, so the next request must wait rather than stack a
	// second frame behind the first.
	p.Request()
	if presented(t, &p, epoch, d.draw) {
		t.Fatal("drew while a frame was still in flight")
	}
	p.Wrote(1)
	if !presented(t, &p, epoch, d.draw) || d.calls != 2 {
		t.Fatalf("drew %d times, want the held request satisfied once acknowledged", d.calls)
	}
}

func TestAFullRepaintRequestSurvivesCoalescing(t *testing.T) {
	var p present.Presenter
	d := drew{seq: 1}
	p.RequestFull()
	p.Request()
	p.Request()
	presented(t, &p, epoch, d.draw)
	if !d.full {
		t.Fatal("plain requests downgraded a full repaint")
	}
	// And it is consumed: the next frame is an ordinary one.
	p.Wrote(1)
	p.Request()
	presented(t, &p, epoch, d.draw)
	if d.full {
		t.Fatal("the full repaint flag outlived the frame that used it")
	}
}

func TestAFrameThatQueuedNothingLeavesNothingInFlight(t *testing.T) {
	var p present.Presenter
	// An unchanged frame writes no bytes, so there is nothing to wait for and the
	// next request must not be held.
	d := drew{seq: 0}
	p.Request()
	presented(t, &p, epoch, d.draw)
	p.Request()
	if !presented(t, &p, epoch, d.draw) {
		t.Fatal("the next request was held behind a frame that does not exist")
	}
}

func TestAcknowledgementOfALaterFrameReleasesAnEarlierOne(t *testing.T) {
	var p present.Presenter
	d := drew{seq: 7}
	p.Request()
	presented(t, &p, epoch, d.draw)
	// The writer reports progress in sequence order, so anything at or past the
	// frame being waited for means it has landed.
	p.Wrote(9)
	p.Request()
	if !presented(t, &p, epoch, d.draw) {
		t.Fatal("still waiting after the writer passed the frame")
	}
}

func TestThrottledRequestsAreSpacedOut(t *testing.T) {
	var p present.Presenter
	d := drew{seq: 1}
	const interval = 16 * time.Millisecond

	// The first request is never throttled: nothing has been drawn to space it
	// away from.
	if !p.RequestBy(epoch, interval) {
		t.Fatal("the first throttled request was turned away")
	}
	presented(t, &p, epoch, d.draw)
	p.Wrote(1)

	if p.RequestBy(epoch.Add(5*time.Millisecond), interval) {
		t.Fatal("a request inside the interval was taken")
	}
	if presented(t, &p, epoch.Add(5*time.Millisecond), d.draw) {
		t.Fatal("a turned-away request still drew a frame")
	}
	due, armed := p.DueAt()
	if !armed || !due.Equal(epoch.Add(interval)) {
		t.Fatalf("due at %v (armed %v), want %v", due, armed, epoch.Add(interval))
	}
	// Asking again inside the interval must not push the deadline out, or a
	// continuous stream of requests would never come due.
	p.RequestBy(epoch.Add(10*time.Millisecond), interval)
	if again, _ := p.DueAt(); !again.Equal(due) {
		t.Fatalf("deadline moved to %v, want it to stay at %v", again, due)
	}

	if !p.RequestBy(epoch.Add(interval), interval) {
		t.Fatal("a request at the deadline was turned away")
	}
	presented(t, &p, epoch.Add(interval), d.draw)
	if _, armed := p.DueAt(); armed {
		t.Fatal("the deadline outlived the frame that satisfied it")
	}
}

func TestClockReversalStartsANewPacingEpoch(t *testing.T) {
	var p present.Presenter
	d := drew{seq: 1}
	const interval = 16 * time.Millisecond

	p.RequestBy(epoch, interval)
	presented(t, &p, epoch, d.draw)
	p.Wrote(1)
	if p.RequestBy(epoch.Add(time.Millisecond), interval) {
		t.Fatal("request inside the old interval was not throttled")
	}

	earlier := epoch.Add(-time.Hour)
	if !presented(t, &p, earlier, d.draw) {
		t.Fatal("an old pacing deadline survived a clock reversal")
	}
	if _, armed := p.DueAt(); armed {
		t.Fatal("a deadline from the old clock epoch remained armed")
	}

	p.Wrote(1)
	if !p.RequestBy(earlier.Add(interval), interval) {
		t.Fatal("the new clock epoch did not establish its own pacing interval")
	}
}

func TestAFrameInFlightHoldsEverythingBehindIt(t *testing.T) {
	// Which is the whole of the backpressure: a terminal that has not finished with the
	// last frame is not given another.
	var p present.Presenter
	d := drew{seq: 1}
	p.Request()
	presented(t, &p, epoch, d.draw)
	for range 5 {
		p.Request()
		if presented(t, &p, epoch, d.draw) {
			t.Fatal("drew while the terminal was still busy with the last frame")
		}
	}
	p.Wrote(1)
	p.Request()
	if !presented(t, &p, epoch, d.draw) {
		t.Fatal("never drew again once the terminal caught up")
	}
}

func TestAThrottledRequestStillOwesAFrame(t *testing.T) {
	// The interval decides when a frame is drawn, not whether. A request that only
	// recorded a deadline would lose the last update of a burst: the loop would wake at
	// the deadline, find nothing owed, and draw nothing.
	var p present.Presenter
	d := drew{seq: 1}
	const interval = 16 * time.Millisecond

	p.RequestBy(epoch, interval)
	presented(t, &p, epoch, d.draw)
	p.Wrote(1)

	if p.RequestBy(epoch.Add(time.Millisecond), interval) {
		t.Fatal("a request inside the interval was allowed straight away")
	}
	if presented(t, &p, epoch.Add(time.Millisecond), d.draw) {
		t.Fatal("a frame was drawn before it was due")
	}
	// At the deadline it is drawn, without anyone having to ask a second time.
	if !presented(t, &p, epoch.Add(interval), d.draw) {
		t.Fatalf("the frame owed since the burst was never drawn")
	}
}

func TestAskingForAFrameNowCancelsTheWait(t *testing.T) {
	// A resize cannot be made to wait behind a token stream's pacing.
	var p present.Presenter
	d := drew{seq: 1}
	const interval = 16 * time.Millisecond

	p.Request()
	presented(t, &p, epoch, d.draw)
	p.Wrote(1)
	p.RequestBy(epoch.Add(time.Millisecond), interval)
	p.RequestFull()
	if !presented(t, &p, epoch.Add(time.Millisecond), d.draw) {
		t.Fatal("a full repaint waited behind a throttled request")
	}
	if !d.full {
		t.Fatal("the frame was not a full repaint")
	}
}

func TestAFailedDrawDoesNotClaimTheFrame(t *testing.T) {
	var p present.Presenter
	p.RequestFull()
	cause := errors.New("painter failed")
	full := false
	drew, err := p.Present(epoch, func(repaint bool) (uint64, error) {
		full = repaint
		return 0, cause
	})
	if drew {
		t.Fatal("a failed draw was reported as presented")
	}
	if !errors.Is(err, cause) {
		t.Fatalf("Present error = %v, want painter cause", err)
	}
	if !full {
		t.Fatal("the failed draw was not given the full-repaint request")
	}

	full = false
	if !presented(t, &p, epoch, func(repaint bool) (uint64, error) {
		full = repaint
		return 1, nil
	}) {
		t.Fatal("the request disappeared after its first draw failed")
	}
	if !full {
		t.Fatal("the full-repaint requirement disappeared after a failed draw")
	}
}
