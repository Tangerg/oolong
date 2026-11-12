package present_test

import (
	"testing"
	"time"

	"github.com/Tangerg/oolong/ui/core/present"
)

func TestPacingReadinessUsesTheEarliestCoalescedDeadline(t *testing.T) {
	const firstInterval = 100 * time.Millisecond
	for _, test := range []struct {
		name     string
		at       time.Duration
		interval time.Duration
		ready    bool
		due      time.Duration
	}{
		{name: "shorter interval", at: 10 * time.Millisecond, interval: 20 * time.Millisecond, due: 20 * time.Millisecond},
		{name: "longer interval", at: 10 * time.Millisecond, interval: 200 * time.Millisecond, due: firstInterval},
		{name: "longer interval at deadline", at: firstInterval, interval: 200 * time.Millisecond, ready: true},
		{name: "longer interval after deadline", at: firstInterval + time.Millisecond, interval: 200 * time.Millisecond, ready: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			var p present.Presenter
			var d drew
			p.Request()
			presented(t, &p, epoch, d.draw)
			if p.RequestBy(epoch.Add(time.Millisecond), firstInterval) {
				t.Fatal("first request ignored its pacing interval")
			}
			at := epoch.Add(test.at)
			if ready := p.RequestBy(at, test.interval); ready != test.ready {
				t.Fatalf("ready = %t, want %t", ready, test.ready)
			}
			due, waiting := p.DueAt()
			if test.ready {
				if waiting {
					t.Fatalf("ready request retained deadline %v", due)
				}
				if !p.RequestBy(at, time.Hour) {
					t.Fatal("a later request delayed the ready frame")
				}
			} else if !waiting || !due.Equal(epoch.Add(test.due)) {
				t.Fatalf("deadline = %v, waiting = %t; want %v", due, waiting, epoch.Add(test.due))
			}
			if got := presented(t, &p, at, d.draw); got != test.ready {
				t.Fatalf("presented = %t, want %t", got, test.ready)
			}
			if !test.ready && !presented(t, &p, due, d.draw) {
				t.Fatal("coalesced frame was not drawn at its deadline")
			}
			if d.calls != 2 {
				t.Fatalf("drew %d frames, want 2", d.calls)
			}
		})
	}
}

func TestPacingReadinessDoesNotReleaseWriterBackpressure(t *testing.T) {
	var p present.Presenter
	d := drew{seq: 1}
	p.Request()
	presented(t, &p, epoch, d.draw)
	p.RequestBy(epoch.Add(time.Millisecond), 10*time.Millisecond)
	at := epoch.Add(10 * time.Millisecond)
	if !p.RequestBy(at, time.Hour) {
		t.Fatal("an expired request was paced by a later interval")
	}
	if _, waiting := p.DueAt(); waiting {
		t.Fatal("writer backpressure armed an unactionable pacing timer")
	}
	if presented(t, &p, at, d.draw) {
		t.Fatal("pacing readiness released the unfinished output")
	}
	p.Wrote(1)
	if !presented(t, &p, at, d.draw) || d.calls != 2 {
		t.Fatal("writer completion did not release the ready frame")
	}
}
