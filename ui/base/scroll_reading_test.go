package base_test

import (
	"testing"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/input"
)

func TestScrollRevealMovesAsLittleAsItCan(t *testing.T) {
	// A reader stepping through search results wants the text around them to stay put
	// where it already fits. Centring every result loses the context that made it
	// worth finding.
	for _, tc := range []struct {
		name       string
		offset     int
		row        int
		wantOffset int
	}{
		{name: "already visible", offset: 10, row: 12, wantOffset: 10},
		{name: "the first visible row", offset: 10, row: 10, wantOffset: 10},
		{name: "the last visible row", offset: 10, row: 14, wantOffset: 10},
		{name: "just above", offset: 10, row: 9, wantOffset: 9},
		{name: "far above", offset: 10, row: 2, wantOffset: 2},
		{name: "just below", offset: 10, row: 15, wantOffset: 11},
		{name: "far below", offset: 10, row: 40, wantOffset: 36},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var s base.Scroll
			stageScroll(&s, 100, 5)
			s.By(tc.offset)
			s.Reveal(tc.row, tc.row)
			if got := s.Offset(); got != tc.wantOffset {
				t.Errorf("offset = %d, want %d", got, tc.wantOffset)
			}
		})
	}
}

func TestScrollRevealStopsFollowing(t *testing.T) {
	// A row was asked for, and following the end would scroll away from it at once.
	var s base.Scroll
	s.ToBottom()
	stageScroll(&s, 100, 5)
	if !s.FollowingEnd() {
		t.Fatal("not following to begin with")
	}
	s.Reveal(3, 3)
	if s.FollowingEnd() {
		t.Error("still following after a row was asked for")
	}
	if got := s.Offset(); got != 3 {
		t.Errorf("offset = %d, want 3", got)
	}
}

func TestScrollRevealShowsWhereReadingBegins(t *testing.T) {
	// A match taller than the window: its start wins, or the reader is shown the end
	// of something and left to scroll back for what it was part of.
	var s base.Scroll
	stageScroll(&s, 100, 3)
	s.Reveal(20, 40)
	if got := s.Offset(); got != 20 {
		t.Errorf("offset = %d, want the start of the range", got)
	}

	// And a range that fits is brought in whole.
	s.Reveal(60, 62)
	if got := s.Offset(); got != 60 {
		t.Errorf("offset = %d, want 60", got)
	}
}

func TestScrollRevealBackwardsRangeIsTheSameRange(t *testing.T) {
	var s base.Scroll
	stageScroll(&s, 100, 5)
	s.Reveal(40, 20)
	if got := s.Offset(); got != 20 {
		t.Errorf("offset = %d, want 20", got)
	}
}

func TestScrollRevealWithNoWindowDoesNothing(t *testing.T) {
	var s base.Scroll
	s.Reveal(50, 50)
	if got := s.Offset(); got != 0 {
		t.Errorf("offset = %d before any layout", got)
	}
}

// TestScrollTakesTheTerminalsWordForWhatANotchIs. The same code used to scroll three
// times as far on Apple Terminal as on iTerm2, because it moved a fixed number of rows
// per report and they send a different number of reports.
func TestScrollTakesTheTerminalsWordForWhatANotchIs(t *testing.T) {
	keys := base.DefaultScrollKeys()
	notch := func(w input.Wheel) int {
		var s base.Scroll
		stageScroll(&s, 1000, 10)
		s.ToTop()
		s.Wheel(w)
		for range w.Reports {
			s.Handle(input.Mouse{Action: input.WheelDown}, keys)
		}
		return s.Offset()
	}

	// One notch is three rows on both, however many reports it took to say so.
	if got := notch(input.Wheel{Reports: 3, Rows: 3}); got != 3 {
		t.Errorf("a notch on a terminal that sends three reports moved %d rows, want 3", got)
	}
	if got := notch(input.Wheel{Reports: 1, Rows: 3}); got != 3 {
		t.Errorf("a notch on a terminal that sends one report moved %d rows, want 3", got)
	}
}
