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
	// The terminal owns what counts as a change, so the loop is driven through one.
	terminal := &Terminal{resized: make(chan input.Resize, 1), size: knownDimensions(80, 24)}
	go func() {
		defer close(done)
		pollResize(stop, ticks, func() (int, int, error) {
			observation := <-observations
			return observation.width, observation.height, observation.err
		}, func(width, height int, err error) {
			terminal.noteResize(width, height, err)
			select {
			case resized := <-terminal.resized:
				reports <- result{width: resized.Width, height: resized.Height}
			default:
			}
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
	terminal.retakeResize(80, 24)
	terminal.retakeResize(100, 30)

	got := <-terminal.resized
	if got.Width != 100 || got.Height != 30 {
		t.Fatalf("queued resize = %dx%d, want newest 100x30", got.Width, got.Height)
	}
}

// TestTakingTheTerminalBackTellsTheWatcherWhatItMissed is the reason the last size
// has one owner.
//
// While a child holds the terminal, its process group gets the resize signals and
// this one does not, so the watcher comes back remembering a size that may not have
// been true for a while. If the measurement taken on the way back is not also the
// watcher's, a change back to the remembered size reads as no change at all — and
// the session goes on drawing at a size the terminal is not.
func TestTakingTheTerminalBackTellsTheWatcherWhatItMissed(t *testing.T) {
	terminal := &Terminal{resized: make(chan input.Resize, 1), size: knownDimensions(80, 24)}

	// The child resized it and nothing here was told.
	terminal.retakeResize(100, 30)
	if got := <-terminal.resized; got != (input.Resize{Width: 100, Height: 30}) {
		t.Fatalf("taking the terminal back reported %+v", got)
	}

	// And now the user puts it back the way it was.
	terminal.noteResize(80, 24, nil)
	select {
	case got := <-terminal.resized:
		if got != (input.Resize{Width: 80, Height: 24}) {
			t.Fatalf("the change back reported %+v", got)
		}
	default:
		t.Fatal("the change back to the size the watcher remembered went unreported")
	}
}
