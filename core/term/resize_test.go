package term

import (
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/oolong/core/input"
)

func TestDimensionsReportOnlySuccessfulChanges(t *testing.T) {
	last := knownDimensions(80, 24)
	if last.observe(80, 24, nil) {
		t.Fatal("the opening size was reported twice")
	}
	if last.observe(0, 0, errors.New("size unavailable")) {
		t.Fatal("a failed observation became a resize")
	}
	if !last.observe(100, 30, nil) {
		t.Fatal("a changed size was not reported")
	}
	if last.observe(100, 30, nil) {
		t.Fatal("an unchanged size was reported after a change")
	}
}

func TestDimensionsReportFirstSizeWhenOpeningSizeWasUnavailable(t *testing.T) {
	var last dimensions
	if last.observe(0, 0, errors.New("size unavailable")) {
		t.Fatal("a failed first observation became a resize")
	}
	if !last.observe(80, 24, nil) {
		t.Fatal("the first available size was not reported")
	}
}

func TestResizePollingReportsChangesAndStops(t *testing.T) {
	type result struct {
		width, height int
		err           error
	}
	observations := make(chan result, 4)
	observations <- result{width: 80, height: 24}
	observations <- result{err: errors.New("temporarily unavailable")}
	observations <- result{width: 100, height: 30}
	observations <- result{width: 100, height: 30}

	stop := make(chan struct{})
	ticks := make(chan time.Time)
	reports := make(chan result, 4)
	done := make(chan struct{})
	go func() {
		defer close(done)
		pollResize(stop, ticks, knownDimensions(80, 24), func() (int, int, error) {
			observation := <-observations
			return observation.width, observation.height, observation.err
		}, func(width, height int) {
			reports <- result{width: width, height: height}
		})
	}()

	tick := func() {
		t.Helper()
		ticks <- time.Time{}
	}
	noReport := func() {
		t.Helper()
		select {
		case <-reports:
			t.Fatal("resize polling reported an unchanged or failed observation")
		default:
		}
	}

	tick() // unchanged
	noReport()
	tick() // failed
	noReport()
	tick() // changed
	select {
	case got := <-reports:
		if got.width != 100 || got.height != 30 {
			t.Fatalf("reported size = %dx%d, want 100x30", got.width, got.height)
		}
	case <-t.Context().Done():
		t.Fatal("resize polling did not report changed geometry")
	}
	tick() // unchanged after the change
	noReport()

	close(stop)
	select {
	case <-done:
	case <-t.Context().Done():
		t.Fatal("resize polling did not stop with its terminal")
	}
}

func TestResizeMailboxKeepsTheNewestObservation(t *testing.T) {
	terminal := &Terminal{resized: make(chan input.Resize, 1)}
	terminal.reportResize(80, 24)
	terminal.reportResize(100, 30)

	got := <-terminal.resized
	if got.Width != 100 || got.Height != 30 {
		t.Fatalf("queued resize = %dx%d, want newest 100x30", got.Width, got.Height)
	}
}
