package term

import (
	"slices"
	"strings"

	"github.com/Tangerg/oolong/ui/core/ansi"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/termenv"
)

// KeyboardCompatible is the portable keyboard enhancement set. It makes modified
// keys unambiguous and reports alternate-layout keycodes without asking for release
// events or turning ordinary text into escape sequences. Applications needing those
// less widely reliable behaviours add their features explicitly.
const KeyboardCompatible = input.KeyboardDisambiguate | input.KeyboardReportAlternates

// Modes is the immutable set of terminal modes a session turns on and later puts
// back. Its fields stay private so the only way to construct one is from [Config],
// keeping feature requests, screen ownership, and the wire representation from
// drifting.
//
// Enter and Leave are encodings rather than writes. A local terminal writes them
// around raw-mode ownership; a transport such as SSH queues them around its own
// session lifecycle.
type Modes struct {
	altScreen       bool
	mouse           bool
	focus           bool
	keyboard        input.KeyboardFeatures
	observeKeyboard bool
	restoreCursor   string
}

// restoringCursor is these modes with the cursor style the terminal reported at
// entry as the restoration target, which a probe learns only after acquisition.
func (m Modes) restoringCursor(style ansi.CursorStyle) Modes {
	m.restoreCursor = style.Sequence()
	return m
}

// Validate refuses a configuration no session can honour. Every transport checks
// it before acquiring a terminal, so a refusal leaves nothing to restore.
func (c Config) Validate() error {
	return c.RestoreCursor.Shape.Validate()
}

// Modes includes keyboard observation in acquisition for both local and remote
// transports so startup probing consumes that reply without a second query. It
// returns the error Validate reports for a configuration no session can honour.
//
// lookup is the environment of the terminal being driven, not necessarily this
// process. A local terminal passes its process environment lookup; an SSH adapter
// passes the client's accepted PTY environment. Nil means no environment facts are
// available.
func (c Config) Modes(env termenv.Lookup) (Modes, error) {
	if err := c.Validate(); err != nil {
		return Modes{}, err
	}
	keyboard := compatibleKeyboard(c.Features.Keyboard, env)
	return Modes{
		altScreen:       c.AltScreen,
		mouse:           c.Features.Mouse,
		focus:           c.Features.Focus,
		keyboard:        keyboard,
		observeKeyboard: keyboard != 0 || c.Features.Probe,
		restoreCursor:   c.RestoreCursor.Sequence(),
	}, nil
}

func compatibleKeyboard(features input.KeyboardFeatures, env termenv.Lookup) input.KeyboardFeatures {
	features &= input.KeyboardAll
	if features.Has(input.KeyboardReportText) {
		features |= input.KeyboardReportAllAsEscapes
	}
	if features == 0 {
		return features
	}

	// VS Code's terminal bridge in WSL can acknowledge progressive keyboard mode
	// and then corrupt or lose the sequences it carries. An application running on
	// another machine must use that session's environment, which is why this decision
	// is made while Modes is built rather than by a package global.
	terminal := env.Terminal()
	if env.WSL() && terminal == termenv.VSCode {
		return 0
	}

	// iTerm2 can leak a release belonging to the exiting application into its parent
	// shell. The compatible set never asks for releases; callers that add them still
	// get every other requested feature on this terminal.
	if terminal == termenv.ITerm2 {
		features &^= input.KeyboardReportEvents
	}
	return features
}

func keyboardOn(features input.KeyboardFeatures) string {
	if features == 0 {
		return ""
	}
	return ansi.PushKeyboard(int(features))
}

// mode pairs acquisition with restoration: terminal modes outlive the process
// and would otherwise affect the next foreground owner.
type mode struct {
	on, off string
	wanted  bool
}

func decMode(m ansi.Mode, wanted bool) mode { return mode{m.Set(), m.Reset(), wanted} }

// Bracketed paste is unconditional so pasted code cannot become shortcuts.
// Mouse reporting pairs any-event tracking with the SGR coordinate encoding.
func (m Modes) sequence() []mode {
	return []mode{
		decMode(ansi.ModeAltScreen, m.altScreen),
		decMode(ansi.ModeAnyEventMouse, m.mouse),
		decMode(ansi.ModeSGRMouse, m.mouse),
		decMode(ansi.ModeFocusReports, m.focus),
		{keyboardOn(m.keyboard), ansi.PopKeyboard, m.keyboard != 0},
		decMode(ansi.ModeBracketedPaste, true),
	}
}

func (m Modes) Enter() string {
	var b strings.Builder
	for _, mode := range m.sequence() {
		if mode.wanted {
			b.WriteString(mode.on)
		}
	}
	if m.observeKeyboard {
		b.WriteString(ansi.QueryKeyboard)
	}
	return b.String()
}

// Leave is what to write to give the terminal back: every mode that was turned on,
// turned off in the opposite order, followed by the chosen cursor restoration
// target and visibility. The zero target is a protocol reset, not a saved shape.
func (m Modes) Leave() string {
	seq := m.sequence()
	var b strings.Builder
	for _, mode := range slices.Backward(seq) {
		if mode.wanted {
			b.WriteString(mode.off)
		}
	}
	restore := m.restoreCursor
	if restore == "" {
		restore = ansi.CursorReset.Sequence()
	}
	b.WriteString(restore)
	b.WriteString(ansi.ModeCursorVisible.Set())
	return b.String()
}
