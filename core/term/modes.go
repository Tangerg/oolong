package term

import (
	"slices"
	"strconv"
	"strings"

	"github.com/Tangerg/oolong/core/input"
)

// Terminal modes, each written as a pair: what turns it on, and what puts it back.
//
// Every one of these outlives the process if it is not put back. A program that
// exits with mouse reporting on leaves the shell printing escape sequences when
// the user moves the mouse; one that exits on the alternate screen loses whatever
// the user had on screen before it started. Restoring them is not tidiness.
const (
	altScreenOn  = "\x1b[?1049h"
	altScreenOff = "\x1b[?1049l"

	// Mouse reporting: any-event tracking so hover is reported and not only drags,
	// plus the extended coordinate encoding, which is the only one that works past
	// column 223.
	mouseOn  = "\x1b[?1003h\x1b[?1006h"
	mouseOff = "\x1b[?1006l\x1b[?1003l"

	focusOn  = "\x1b[?1004h"
	focusOff = "\x1b[?1004l"

	keyboardOff = "\x1b[<u"

	pasteOn  = "\x1b[?2004h"
	pasteOff = "\x1b[?2004l"

	cursorDefault = "\x1b[0 q"
	cursorShow    = "\x1b[?25h"
)

// KeyboardCompatible is the portable keyboard enhancement set. It makes modified
// keys unambiguous and reports alternate-layout keycodes without asking for release
// events or turning ordinary text into escape sequences. Applications needing those
// less widely reliable behaviours add their features explicitly.
const KeyboardCompatible = input.KeyboardDisambiguate | input.KeyboardReportAlternates

// Modes is the immutable set of terminal modes a session turns on and later puts
// back. Its fields stay private so the only way to construct one is from [Options],
// keeping the public configuration and the wire representation from drifting.
//
// Enter and Leave are encodings rather than writes. A local terminal writes them
// around raw-mode ownership; a transport such as SSH queues them around its own
// session lifecycle.
type Modes struct {
	altScreen bool
	mouse     bool
	focus     bool
	keyboard  input.KeyboardFeatures
}

// Modes returns the terminal-mode encoding selected by o for an environment.
// Probe is deliberately absent from the result: probing is an input round trip,
// not an output mode.
//
// lookup is the environment of the terminal being driven, not necessarily this
// process. A local terminal passes its process environment lookup; an SSH adapter
// passes the client's accepted PTY environment. Nil means no environment facts are
// available.
func (o Options) Modes(lookup func(string) (string, bool)) Modes {
	return Modes{
		altScreen: o.AltScreen,
		mouse:     o.Mouse,
		focus:     o.Focus,
		keyboard:  compatibleKeyboard(o.Keyboard, lookup),
	}
}

func compatibleKeyboard(
	features input.KeyboardFeatures,
	lookup func(string) (string, bool),
) input.KeyboardFeatures {
	features &= input.KeyboardAll
	if features == 0 || lookup == nil {
		return features
	}

	// VS Code's terminal bridge in WSL can acknowledge progressive keyboard mode
	// and then corrupt or lose the sequences it carries. An application running on
	// another machine must use that session's environment, which is why this decision
	// is made while Modes is built rather than by a package global.
	wsl := environmentSet(lookup, "WSL_INTEROP") || environmentSet(lookup, "WSL_DISTRO_NAME")
	vscode := environmentEqual(lookup, "TERM_PROGRAM", "vscode") ||
		environmentSet(lookup, "VSCODE_INJECTION")
	if wsl && vscode {
		return 0
	}

	// iTerm2 can leak a release belonging to the exiting application into its parent
	// shell. The compatible set never asks for releases; callers that add them still
	// get every other requested feature on this terminal.
	if environmentEqual(lookup, "TERM_PROGRAM", "iTerm.app") {
		features &^= input.KeyboardReportEvents
	}
	return features
}

func environmentSet(lookup func(string) (string, bool), name string) bool {
	value, ok := lookup(name)
	return ok && strings.TrimSpace(value) != ""
}

func environmentEqual(lookup func(string) (string, bool), name, want string) bool {
	value, ok := lookup(name)
	return ok && strings.EqualFold(strings.TrimSpace(value), want)
}

func keyboardOn(features input.KeyboardFeatures) string {
	if features == 0 {
		return ""
	}
	return "\x1b[>" + strconv.Itoa(int(features)) + "u"
}

// mode pairs one mode's enable and disable sequences with whether it is wanted.
type mode struct {
	on, off string
	wanted  bool
}

// sequence lists the modes in the order they are turned on. Bracketed paste is
// always wanted: a terminal that cannot tell a paste from typing turns pasted code
// into keystrokes, and there is no reason to want that.
func (m Modes) sequence() []mode {
	return []mode{
		{altScreenOn, altScreenOff, m.altScreen},
		{mouseOn, mouseOff, m.mouse},
		{focusOn, focusOff, m.focus},
		{keyboardOn(m.keyboard), keyboardOff, m.keyboard != 0},
		{pasteOn, pasteOff, true},
	}
}

// enter is what to write to take the terminal over.
func (m Modes) enter() string {
	var b strings.Builder
	for _, mode := range m.sequence() {
		if mode.wanted {
			b.WriteString(mode.on)
		}
	}
	return b.String()
}

// leave is what to write to give the terminal back: every mode that was turned on,
// turned off in the opposite order, and then the cursor restored and shown, because
// a frame may have changed its shape or hidden it.
func (m Modes) leave() string {
	seq := m.sequence()
	var b strings.Builder
	for _, mode := range slices.Backward(seq) {
		if mode.wanted {
			b.WriteString(mode.off)
		}
	}
	b.WriteString(cursorDefault)
	b.WriteString(cursorShow)
	return b.String()
}

// Enter encodes the modes in acquisition order.
func (m Modes) Enter() string { return m.enter() }

// Leave encodes the inverse modes in reverse order and restores the cursor.
func (m Modes) Leave() string { return m.leave() }
