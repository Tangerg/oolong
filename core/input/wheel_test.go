package input_test

import (
	"testing"

	"github.com/Tangerg/oolong/core/input"
)

func env(vars map[string]string) func(string) string {
	return func(name string) string { return vars[name] }
}

// TestWheelForKnowsWhoSendsWhat is the whole problem. A wheel report carries no
// magnitude, so a fixed number of rows per report moves three times as far on one
// terminal as on another and there is no way to ask.
func TestWheelForKnowsWhoSendsWhat(t *testing.T) {
	for _, tc := range []struct {
		name string
		env  map[string]string
		want input.Wheel
	}{
		{name: "nothing said", env: map[string]string{}, want: input.Wheel{}},
		{
			name: "an unknown terminal",
			env:  map[string]string{"TERM": "xterm-256color"},
			want: input.Wheel{},
		},

		// One report to a notch.
		{
			name: "iTerm2",
			env:  map[string]string{"TERM_PROGRAM": "iTerm.app"},
			want: input.Wheel{Reports: 1, Rows: 3},
		},
		{
			name: "iTerm2 across an ssh hop",
			env:  map[string]string{"LC_TERMINAL": "iTerm2"},
			want: input.Wheel{Reports: 1, Rows: 3},
		},
		{
			name: "an editor's embedded terminal",
			env:  map[string]string{"TERM_PROGRAM": "vscode"},
			want: input.Wheel{Reports: 1, Rows: 3},
		},

		// Three reports to a notch.
		{
			name: "Apple Terminal",
			env:  map[string]string{"TERM_PROGRAM": "Apple_Terminal"},
			want: input.Wheel{Reports: 3, Rows: 3},
		},
		{
			name: "kitty by its own variable",
			env:  map[string]string{"KITTY_WINDOW_ID": "1"},
			want: input.Wheel{Reports: 3, Rows: 3},
		},
		{
			name: "alacritty by its socket",
			env:  map[string]string{"ALACRITTY_SOCKET": "/tmp/x"},
			want: input.Wheel{Reports: 3, Rows: 3},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := input.WheelFor(env(tc.env)); got != tc.want {
				t.Errorf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

// TestAMultiplexerAnswersForItself. It reads the reports and writes its own, so
// whatever the outer terminal batched is gone — and its answer replaces rather than
// combines, which is why the check has to come first.
func TestAMultiplexerAnswersForItself(t *testing.T) {
	for _, tc := range []struct {
		name string
		env  map[string]string
	}{
		{name: "tmux by its variable", env: map[string]string{"TMUX": "/tmp/x", "TERM_PROGRAM": "Apple_Terminal"}},
		{name: "tmux by TERM_PROGRAM", env: map[string]string{"TERM_PROGRAM": "tmux"}},
		{name: "screen", env: map[string]string{"STY": "1.pts", "KITTY_WINDOW_ID": "1"}},
		{name: "screen by TERM", env: map[string]string{"TERM": "screen-256color"}},
		{name: "zellij", env: map[string]string{"ZELLIJ": "0", "TERM_PROGRAM": "Apple_Terminal"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			want := input.Wheel{Reports: 1, Rows: 3}
			if got := input.WheelFor(env(tc.env)); got != want {
				t.Errorf("got %+v, want %+v", got, want)
			}
		})
	}
}

func TestWheelForWithNothingToAsk(t *testing.T) {
	if got := input.WheelFor(nil); got != (input.Wheel{}) {
		t.Errorf("got %+v, want the zero value", got)
	}
}

// TestTheZeroWheelIsTheCommonArrangement, which is right on most terminals and wrong
// by at most a factor of three on the rest.
func TestTheZeroWheelIsTheCommonArrangement(t *testing.T) {
	var w input.Wheel
	if got := w.Distance(); got != 1 {
		t.Errorf("a report is worth %v rows, want 1", got)
	}
	// A field left at zero falls back on its own, so a caller can set one and not
	// the other.
	if got := (input.Wheel{Reports: 1}).Distance(); got != 3 {
		t.Errorf("one report to a notch is worth %v rows, want 3", got)
	}
	if got := (input.Wheel{Rows: 6}).Distance(); got != 2 {
		t.Errorf("a six-row notch over three reports is worth %v each, want 2", got)
	}
	// Nonsense falls back rather than dividing by zero or scrolling backwards.
	if got := (input.Wheel{Reports: -4, Rows: -9}).Distance(); got != 1 {
		t.Errorf("got %v, want the fallback", got)
	}
}

// TestAdvanceKeepsWhatAReportWasWorth is why this is a type and not a division. On a
// terminal that sends three reports to a notch worth one row, each is worth a third,
// and rounding each to zero would mean the view never moved while the wheel turned.
func TestAdvanceKeepsWhatAReportWasWorth(t *testing.T) {
	var a input.Advance
	a.Wheel(input.Wheel{Reports: 3, Rows: 1})

	got := []int{a.By(1), a.By(1), a.By(1), a.By(1)}
	want := []int{0, 0, 1, 0}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("report by report: %v, want %v — a third of a row each", got, want)
		}
	}
}

func TestAdvanceUpAndDown(t *testing.T) {
	var a input.Advance
	a.Wheel(input.Wheel{Reports: 1, Rows: 3})
	if got := a.By(-1); got != -3 {
		t.Errorf("one report up = %d rows, want -3", got)
	}
	if got := a.By(1); got != 3 {
		t.Errorf("one report down = %d rows, want 3", got)
	}
}

// TestAdvanceDoesNotSpendARowItDidNotEarn. Truncating towards zero means a change of
// direction cannot cross the boundary and hand out a row that was never turned.
func TestAdvanceDoesNotSpendARowItDidNotEarn(t *testing.T) {
	var a input.Advance
	a.Wheel(input.Wheel{Reports: 3, Rows: 1})

	// Two thirds down, then two thirds back up: nothing moved, and nothing is owed.
	total := a.By(1) + a.By(1) + a.By(-1) + a.By(-1)
	if total != 0 {
		t.Errorf("scrolled %d rows for a gesture that came back to where it began", total)
	}
	// And the next full notch is still a whole row.
	if got := a.By(3); got != 1 {
		t.Errorf("a notch after that = %d rows, want 1", got)
	}
}

func TestAdvanceReset(t *testing.T) {
	var a input.Advance
	a.Wheel(input.Wheel{Reports: 3, Rows: 1})
	a.By(1)
	a.By(1)
	a.Reset()
	if got := a.By(1); got != 0 {
		t.Errorf("after a reset the first report gave %d rows, want 0", got)
	}
}

func TestTheZeroAdvanceScrollsOneRowAReport(t *testing.T) {
	var a input.Advance
	if got := a.By(1); got != 1 {
		t.Errorf("got %d, want one row", got)
	}
}
