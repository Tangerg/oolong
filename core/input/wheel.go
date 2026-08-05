package input

import "strings"

// Wheel says what a terminal's wheel reports are worth.
//
// # Why this is not a constant
//
// A wheel report carries a direction and no magnitude, and terminals disagree about
// how many reports one notch of the wheel is. Apple Terminal, kitty, Ghostty and
// alacritty send three; iTerm2 and WezTerm send one; an editor's embedded terminal
// sends one and means three rows by it. So the same code, scrolling a fixed number of
// rows per report, moves three times as far on one terminal as on another — and there
// is no way to ask, because the protocol does not carry it.
//
// The zero value is the commoner arrangement: three reports to a notch, three rows to
// a notch, which comes to one row a report. It is a reasonable answer everywhere and
// the right one on most terminals.
type Wheel struct {
	// Reports is how many wheel events the terminal sends for one physical notch.
	Reports int
	// Rows is how far one notch should move a view.
	Rows int
}

// wheelDefault is what a terminal that says nothing is assumed to do, and what every
// field left at zero falls back to. Three and three is the commonest pair.
const wheelDefault = 3

// Distance is how many rows one report is worth, as a fraction of a row.
//
// A fraction, because a report is very often worth less than a row and rounding each
// one to zero would stop the view moving at all — see [Wheel.Advance].
func (w Wheel) Distance() float64 {
	reports, rows := w.Reports, w.Rows
	if reports <= 0 {
		reports = wheelDefault
	}
	if rows <= 0 {
		rows = wheelDefault
	}
	return float64(rows) / float64(reports)
}

// wheelProfiles are the terminals worth knowing about, matched as lowercase
// substrings of what they call themselves.
//
// Conservative on purpose: a terminal not on the list gets the common arrangement,
// which is wrong by at most a factor of three, while a wrong entry is wrong for
// everyone who uses that terminal and looks like the library being broken.
var wheelProfiles = []struct {
	name  string
	wheel Wheel
}{
	// One report to a notch, and a notch is three rows.
	{"iterm", Wheel{Reports: 1, Rows: 3}},
	{"wezterm", Wheel{Reports: 1, Rows: 3}},
	{"mintty", Wheel{Reports: 1, Rows: 3}},
	// Editors embedding a terminal send one report and mean a notch by it, and their
	// notch is larger because the pane is small.
	{"vscode", Wheel{Reports: 1, Rows: 3}},
	{"cursor", Wheel{Reports: 1, Rows: 3}},
	{"windsurf", Wheel{Reports: 1, Rows: 3}},
	{"zed", Wheel{Reports: 1, Rows: 3}},
	// Three reports to a notch, which is the common arrangement and the default. They
	// are listed anyway so that the table says what is known rather than only what is
	// unusual.
	{"apple_terminal", Wheel{Reports: 3, Rows: 3}},
	{"ghostty", Wheel{Reports: 3, Rows: 3}},
	{"kitty", Wheel{Reports: 3, Rows: 3}},
	{"alacritty", Wheel{Reports: 3, Rows: 3}},
	{"warp", Wheel{Reports: 3, Rows: 3}},
	{"rio", Wheel{Reports: 3, Rows: 3}},
}

// WheelFor is what the terminal this environment describes does with its wheel.
//
// # Multiplexers
//
// A multiplexer reads the mouse reports and writes its own, so whatever the outer
// terminal batched is gone by the time the program sees anything: tmux, screen and
// zellij all forward one report per notch regardless of what arrived. Their answer
// therefore replaces the outer terminal's rather than being combined with it, and
// checking for them has to come first.
//
// The lookup is passed in rather than read, for the same reason it is everywhere else
// in this library: this package is a function of its inputs, and a test that could not
// say what terminal it was in could not check any of these answers.
func WheelFor(getenv func(string) string) Wheel {
	if getenv == nil {
		return Wheel{}
	}
	if multiplexed(getenv) {
		return Wheel{Reports: 1, Rows: 3}
	}
	identity := strings.ToLower(strings.Join([]string{
		getenv("TERM"), getenv("TERM_PROGRAM"), getenv("LC_TERMINAL"),
	}, " "))
	if getenv("KITTY_WINDOW_ID") != "" || getenv("GHOSTTY_RESOURCES_DIR") != "" ||
		getenv("ALACRITTY_SOCKET") != "" {
		return Wheel{Reports: 3, Rows: 3}
	}
	for _, profile := range wheelProfiles {
		if strings.Contains(identity, profile.name) {
			return profile.wheel
		}
	}
	return Wheel{}
}

// multiplexed reports whether something between the terminal and this program is
// rewriting the mouse reports.
func multiplexed(getenv func(string) string) bool {
	if getenv("TMUX") != "" || getenv("STY") != "" ||
		getenv("ZELLIJ") != "" || getenv("ZELLIJ_SESSION_NAME") != "" {
		return true
	}
	term := strings.ToLower(getenv("TERM"))
	program := strings.ToLower(getenv("TERM_PROGRAM"))
	return program == "tmux" || strings.HasPrefix(term, "screen") ||
		strings.HasPrefix(term, "tmux")
}

// Advance turns a run of wheel reports into whole rows, keeping what is left over.
//
// The remainder is the whole reason this is a type and not a division. On a terminal
// that sends three reports to a notch worth three rows, each report is worth exactly
// one and nothing is left; on one that sends three reports to a notch worth one row,
// each is worth a third, and rounding each to zero would mean the view never moved at
// all while the wheel turned.
//
// Zero rows is an ordinary answer. A caller scrolls by what it gets and asks again on
// the next report.
type Advance struct {
	wheel   Wheel
	carried float64
}

// Wheel replaces what the reports are worth, keeping any part of a row already
// accumulated. It is what a caller does once, having asked [WheelFor].
func (a *Advance) Wheel(w Wheel) { a.wheel = w }

// By is how many rows n reports in one direction come to.
//
// A negative count is upwards, which is what the caller already has: a wheel event is
// one report in a direction, so this is called with plus or minus one.
func (a *Advance) By(reports int) int {
	a.carried += float64(reports) * a.wheel.Distance()
	// Truncated towards zero, so a direction change never crosses the boundary and
	// spends a row it did not earn.
	rows := int(a.carried)
	a.carried -= float64(rows)
	return rows
}

// Reset drops any part of a row accumulated, which a caller does when the view moves
// for a reason that had nothing to do with the wheel.
func (a *Advance) Reset() { a.carried = 0 }
