package input

import (
	"strings"
	"time"
)

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
	// Trackpad is how far a notch's worth of continuous scrolling should move it.
	//
	// A finger is not a notch. A terminal that coalesces a swipe into a few reports
	// has to make each worth more, or a swipe crawls; one that reports every scrap of
	// motion has to make each worth less, or a swipe flings. The two cannot be told
	// apart from a report — only from how fast they arrive, which is why a mouse
	// event carries when it came.
	//
	// It is divided by [trackpadReports] rather than by Reports, because a notch is
	// what Reports counts and a finger does not have notches.
	Trackpad int
}

// trackpadReports is how many reports a notch's worth of finger motion is taken to be.
//
// It is a constant rather than a per-terminal number because the thing it divides
// already is one: what varies between terminals is how much motion they coalesce into
// a report, and that is what Trackpad says.
const trackpadReports = 3

// wheelDefault is what a terminal that says nothing is assumed to do, and what every
// field left at zero falls back to. Three and three is the commonest pair.
const wheelDefault = 3

// Distance is how many rows one report of the wheel is worth, as a fraction of a row.
//
// A fraction, because a report is very often worth less than a row and rounding each
// one to zero would stop the view moving at all — see [Advance].
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

// TrackpadDistance is how many rows one report of continuous scrolling is worth.
//
// A terminal that says nothing about it is taken to treat a finger like the wheel,
// which is what nearly all of them do: the ones where it differs are the ones that
// coalesce a swipe hardest.
func (w Wheel) TrackpadDistance() float64 {
	if w.Trackpad <= 0 {
		return w.Distance()
	}
	return float64(w.Trackpad) / trackpadReports
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
	// One report to a notch, and a notch is one row. A terminal that reports every
	// notch separately is reporting finely, and matching it row for row is what makes
	// the wheel feel the same on it as on one that batches.
	{"iterm", Wheel{Reports: 1, Rows: 1, Trackpad: 3}},
	{"wezterm", Wheel{Reports: 1, Rows: 1, Trackpad: 3}},
	{"mintty", Wheel{Reports: 1, Rows: 1, Trackpad: 3}},
	// An editor embedding a terminal sends one report and means a notch by it, and
	// coalesces a swipe hardest of all — which is why its finger number is the one
	// that is nothing like its wheel number.
	{"vscode", Wheel{Reports: 1, Rows: 3, Trackpad: 15}},
	{"cursor", Wheel{Reports: 1, Rows: 3, Trackpad: 15}},
	{"windsurf", Wheel{Reports: 1, Rows: 3, Trackpad: 15}},
	{"zed", Wheel{Reports: 1, Rows: 3, Trackpad: 3}},
	// Three reports to a notch, which is the common arrangement and the default. On
	// these the two numbers come to the same thing, and the telling apart is moot.
	{"apple_terminal", Wheel{Reports: 3, Rows: 3, Trackpad: 3}},
	{"ghostty", Wheel{Reports: 3, Rows: 3, Trackpad: 3}},
	{"kitty", Wheel{Reports: 3, Rows: 3, Trackpad: 3}},
	{"alacritty", Wheel{Reports: 3, Rows: 3, Trackpad: 3}},
	{"warp", Wheel{Reports: 3, Rows: 3, Trackpad: 3}},
	{"rio", Wheel{Reports: 3, Rows: 3, Trackpad: 3}},
}

// WheelFor is what a terminal does with its wheel.
//
// name is what the terminal said it was when asked, and outranks everything else: an
// environment describes the terminal a session was started from, which over ssh, in a
// container, or under a multiplexer is not the terminal it is talking to. An empty name
// means nothing was asked, or nothing answered, and the environment is all there is.
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
func WheelFor(getenv func(string) string, name string) Wheel {
	if wheel, ok := profileOf(name); ok {
		return wheel
	}
	if getenv == nil {
		return Wheel{}
	}
	if multiplexed(getenv) {
		return Wheel{Reports: 1, Rows: 1, Trackpad: 3}
	}
	if getenv("KITTY_WINDOW_ID") != "" || getenv("GHOSTTY_RESOURCES_DIR") != "" ||
		getenv("ALACRITTY_SOCKET") != "" {
		return Wheel{Reports: 3, Rows: 3, Trackpad: 3}
	}
	if wheel, ok := profileOf(strings.Join([]string{
		getenv("TERM"), getenv("TERM_PROGRAM"), getenv("LC_TERMINAL"),
	}, " ")); ok {
		return wheel
	}
	return Wheel{}
}

// profileOf is the profile of whichever terminal an identity names.
func profileOf(identity string) (Wheel, bool) {
	if identity == "" {
		return Wheel{}, false
	}
	identity = strings.ToLower(identity)
	for _, profile := range wheelProfiles {
		if strings.Contains(identity, profile.name) {
			return profile.wheel, true
		}
	}
	return Wheel{}, false
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

	// The current gesture: when it began, when its last report came, and how many it
	// has had. A gesture is a run of reports with no gap in it.
	began, last time.Time
	reports     int
	finger      bool
}

const (
	// gestureGap ends a gesture. Anything arriving later than this is a new one, which
	// on a wheel means the next notch and on a finger means the next swipe.
	gestureGap = 200 * time.Millisecond

	// fingerInterval is how fast reports have to arrive to be a finger rather than a
	// wheel, and fingerReports how many of them it takes to be sure.
	//
	// Only a terminal that sends one report per notch can be asked this: on one that
	// sends three, a notch already looks like a burst, and the two numbers it would be
	// asked to choose between come to the same thing anyway.
	fingerInterval = 30 * time.Millisecond
	fingerReports  = 2
)

// Wheel replaces what the reports are worth, keeping any part of a row already
// accumulated. It is what a caller does once, having asked [WheelFor].
func (a *Advance) Wheel(w Wheel) { a.wheel = w }

// By is how many rows n reports in one direction come to.
//
// A negative count is upwards, which is what the caller already has: a wheel event is
// one report in a direction, so this is called with plus or minus one.
func (a *Advance) By(reports int) int {
	return a.at(time.Time{}, reports)
}

// At is the same for a report that came with a time on it, which is what a terminal's
// reader stamps — see [Mouse.At].
//
// The time is what tells a finger from the wheel. Both send the same report, and only
// how fast they arrive is different: a wheel's notches come as far apart as a hand can
// turn them, and a finger's motion arrives as fast as the terminal can report it.
func (a *Advance) At(when time.Time, reports int) int { return a.at(when, reports) }

func (a *Advance) at(when time.Time, reports int) int {
	distance := a.wheel.Distance()
	if !when.IsZero() {
		if a.last.IsZero() || when.Sub(a.last) > gestureGap {
			a.began, a.reports, a.finger = when, 0, false
		}
		a.last = when
		a.reports += abs(reports)
		// A gesture is a finger once it has sent more reports, faster, than a hand
		// could turn a wheel. It stays one until the gesture ends, because a finger
		// slowing to a stop is still a finger.
		if !a.finger && a.wheel.Reports <= 1 && a.reports > fingerReports {
			if elapsed := when.Sub(a.began); elapsed <= time.Duration(a.reports)*fingerInterval {
				a.finger = true
			}
		}
		if a.finger {
			distance = a.wheel.TrackpadDistance()
		}
	}
	a.carried += float64(reports) * distance
	// Truncated towards zero, so a direction change never crosses the boundary and
	// spends a row it did not earn.
	rows := int(a.carried)
	a.carried -= float64(rows)
	return rows
}

// Reset drops any part of a row accumulated and forgets the gesture, which a caller
// does when the view moves for a reason that had nothing to do with scrolling.
func (a *Advance) Reset() {
	a.carried = 0
	a.last, a.began, a.reports, a.finger = time.Time{}, time.Time{}, 0, false
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
