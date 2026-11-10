package input

import (
	"strings"
	"time"
)

// Wheel compensates for terminal-specific report batching, which the mouse
// protocol does not describe. The zero value uses three reports and three rows
// per notch; explicit profiles can override that assumption.
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

// wheelDefault supplies omitted report and row settings consistently.
const wheelDefault = 3

// Distance is how many rows one report of the wheel is worth, as a fraction of a row.
//
// A fraction, because a report is very often worth less than a row and rounding each
// one to zero would stop the view moving at all — see [WheelAccumulator].
func (w Wheel) Distance() float64 {
	reports, rows := w.effectiveReports(), w.Rows
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

// The batching families terminals fall into. Naming each once keeps the identity
// table and the environment hints from describing the same terminal twice.
var (
	// One report to a notch, and a notch is one row. A terminal that reports every
	// notch separately is reporting finely, and matching it row for row is what makes
	// the wheel feel the same on it as on one that batches.
	perNotchWheel = Wheel{Reports: 1, Rows: 1, Trackpad: 3}
	// An editor embedding a terminal sends one report and means a notch by it, and
	// coalesces a swipe hardest of all — which is why its finger number is the one
	// that is nothing like its wheel number.
	embeddedWheel  = Wheel{Reports: 1, Rows: 3, Trackpad: 15}
	notchRowsWheel = Wheel{Reports: 1, Rows: 3, Trackpad: 3}
	batchedWheel   = Wheel{Reports: 3, Rows: 3, Trackpad: 3}
)

// Keep wheel profiles conservative: a false identity match applies the wrong
// batching to every gesture, while unknown terminals can retain the default.
var wheelProfiles = []struct {
	name  string
	wheel Wheel
}{
	{"iterm", perNotchWheel},
	{"wezterm", perNotchWheel},
	{"mintty", perNotchWheel},
	{"vscode", embeddedWheel},
	{"cursor", embeddedWheel},
	{"windsurf", embeddedWheel},
	{"zed", notchRowsWheel},
	{"apple_terminal", batchedWheel},
	{"ghostty", batchedWheel},
	{"kitty", batchedWheel},
	{"alacritty", batchedWheel},
	{"warp", batchedWheel},
	{"rio", batchedWheel},
}

// WheelFor chooses wheel batching from the negotiated terminal and environment.
// Negotiated names outrank potentially remote environment values. Multiplexers
// replace outer-terminal batching because they emit one report per notch. The
// lookup is explicit so each session and test supplies its own environment.
func WheelFor(lookup func(string) (string, bool), name string) Wheel {
	return environmentWheel(lookup).Negotiated(name)
}

// Negotiated applies a terminal's reported identity to an already measured
// environment profile. An unrecognized or absent report preserves that profile.
func (w Wheel) Negotiated(name string) Wheel {
	if wheel, ok := profileOf(name); ok {
		return wheel
	}
	return w
}

func environmentWheel(lookup func(string) (string, bool)) Wheel {
	if lookup == nil {
		return Wheel{}
	}
	if multiplexed(lookup) {
		return perNotchWheel
	}
	if environmentValue(lookup, "KITTY_WINDOW_ID") != "" ||
		environmentValue(lookup, "GHOSTTY_RESOURCES_DIR") != "" ||
		environmentValue(lookup, "ALACRITTY_SOCKET") != "" {
		return batchedWheel
	}
	if wheel, ok := profileOf(strings.Join([]string{
		environmentValue(lookup, "TERM"),
		environmentValue(lookup, "TERM_PROGRAM"),
		environmentValue(lookup, "LC_TERMINAL"),
	}, " ")); ok {
		return wheel
	}
	return Wheel{}
}

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

func multiplexed(lookup func(string) (string, bool)) bool {
	if environmentValue(lookup, "TMUX") != "" || environmentValue(lookup, "STY") != "" ||
		environmentValue(lookup, "ZELLIJ") != "" ||
		environmentValue(lookup, "ZELLIJ_SESSION_NAME") != "" {
		return true
	}
	term := strings.ToLower(environmentValue(lookup, "TERM"))
	program := strings.ToLower(environmentValue(lookup, "TERM_PROGRAM"))
	return program == "tmux" || strings.HasPrefix(term, "screen") ||
		strings.HasPrefix(term, "tmux")
}

func environmentValue(lookup func(string) (string, bool), name string) string {
	value, _ := lookup(name)
	return value
}

// WheelAccumulator converts wheel reports into rows, retaining fractional rows so
// reports worth less than one row still produce movement after accumulating. Zero
// rows is a valid intermediate result.
type WheelAccumulator struct {
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

// SetWheel replaces what the reports are worth, keeping any part of a row already
// accumulated. It is what a caller does once, having asked [WheelFor].
func (a *WheelAccumulator) SetWheel(w Wheel) { a.wheel = w }

// Accumulate adds reports in one direction at when and returns the whole rows
// they complete.
//
// The time is what tells a finger from the wheel. Both send the same report, and only
// how fast they arrive is different: a wheel's notches come as far apart as a hand can
// turn them, and a finger's motion arrives as fast as the terminal can report it. A
// zero time deliberately disables temporal classification for synthetic input.
//
// A negative report count is upwards, which is what a caller already has: a wheel
// event is one report in a direction, so this is normally called with plus or minus
// one.
func (a *WheelAccumulator) Accumulate(when time.Time, reports int) int {
	distance := a.wheel.Distance()
	if a.observe(when, reports) {
		distance = a.wheel.TrackpadDistance()
	}
	a.carried += float64(reports) * distance
	// Truncated towards zero, so a direction change never crosses the boundary and
	// spends a row it did not earn.
	rows, remainder := wholeRows(a.carried)
	a.carried = remainder
	return rows
}

// observe advances gesture identity and reports whether the current reports come
// from a finger. Classification belongs to the gesture state itself; converting
// reports to rows only consumes its answer.
func (a *WheelAccumulator) observe(when time.Time, reports int) bool {
	if when.IsZero() {
		return false
	}
	gap := when.Sub(a.last)
	if a.last.IsZero() || gap < 0 || gap > gestureGap {
		a.began, a.reports, a.finger = when, 0, false
	}
	a.last = when
	a.reports = addReportCount(a.reports, reports)
	// A gesture is a finger once it has sent more reports, faster, than a hand
	// could turn a wheel. It stays one until the gesture ends, because a finger
	// slowing to a stop is still a finger.
	if !a.finger && a.wheel.effectiveReports() == 1 && a.reports > fingerReports {
		a.finger = withinReportRate(when.Sub(a.began), a.reports)
	}
	return a.finger
}

// Reset drops any part of a row accumulated and forgets the gesture, which a caller
// does when the view moves for a reason that had nothing to do with scrolling.
func (a *WheelAccumulator) Reset() {
	a.carried = 0
	a.last, a.began, a.reports, a.finger = time.Time{}, time.Time{}, 0, false
}

const (
	maxInt = int(^uint(0) >> 1)
	minInt = -maxInt - 1
)

// addReportCount records magnitude rather than direction and saturates because the
// count is evidence for classifying one gesture, not a quantity worth wrapping.
func addReportCount(total, reports int) int {
	magnitude := reports
	if reports < 0 {
		if reports == minInt {
			magnitude = maxInt
		} else {
			magnitude = -reports
		}
	}
	if magnitude > maxInt-total {
		return maxInt
	}
	return total + magnitude
}

// withinReportRate compares elapsed <= reports*interval without overflowing the
// duration multiplication at an extreme public input.
func withinReportRate(elapsed time.Duration, reports int) bool {
	if elapsed < 0 || reports <= 0 {
		return false
	}
	whole := elapsed / fingerInterval
	allowed := time.Duration(reports)
	return whole < allowed || whole == allowed && elapsed%fingerInterval == 0
}

// wholeRows turns the floating remainder into an int without letting a very large
// public report count reverse direction at the conversion boundary.
func wholeRows(distance float64) (int, float64) {
	if distance >= float64(maxInt) {
		return maxInt, 0
	}
	if distance <= float64(minInt) {
		return minInt, 0
	}
	rows := int(distance)
	return rows, distance - float64(rows)
}

func (w Wheel) effectiveReports() int {
	if w.Reports <= 0 {
		return wheelDefault
	}
	return w.Reports
}
