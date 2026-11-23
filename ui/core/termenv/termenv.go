// Package termenv owns how a terminal is identified from its session environment.
// That environment may belong to a remote PTY rather than this process, so every
// reading goes through an explicit [Lookup]. Consumers ask their own questions —
// wheel batching, image protocol, keyboard compatibility — of the same evidence.
package termenv

import (
	"strings"

	"github.com/Tangerg/oolong/ui/internal/enum"
)

// Lookup reads one variable of the driven terminal's environment, in the shape of
// os.LookupEnv. A nil Lookup is an environment that makes no claims.
type Lookup func(name string) (string, bool)

// Value is name's value, or empty when it is unset.
func (l Lookup) Value(name string) string {
	if l == nil {
		return ""
	}
	value, _ := l(name)
	return value
}

// Set reports a non-empty value; an exported empty variable makes no claim,
// which is also the NO_COLOR convention.
func (l Lookup) Set(name string) bool { return l.Value(name) != "" }

// Is compares name's value case-insensitively, because terminals disagree on
// the capitalization of the names they export.
func (l Lookup) Is(name, want string) bool { return strings.EqualFold(l.Value(name), want) }

// Term is the terminfo name in lower case. Its spelling is a family convention
// rather than a registry, so consumers compare prefixes and substrings of it.
func (l Lookup) Term() string { return strings.ToLower(l.Value("TERM")) }

// Identity joins the variables through which terminals name themselves, in lower
// case. Spellings differ by terminal and transport — "iTerm.app" in TERM_PROGRAM,
// "iTerm2" in LC_TERMINAL — so [Recognize] matches identifying substrings of it.
func (l Lookup) Identity() string {
	return l.Term() + " " + strings.ToLower(l.Value("TERM_PROGRAM")+" "+l.Value("LC_TERMINAL"))
}

// Terminal identifies the emulator from its native markers before its names,
// because a marker survives a TERM that ssh or a configuration has rewritten.
func (l Lookup) Terminal() Terminal {
	switch {
	case l.Set("KITTY_WINDOW_ID"):
		return Kitty
	case l.Set("GHOSTTY_RESOURCES_DIR"):
		return Ghostty
	case l.Set("ALACRITTY_SOCKET"):
		return Alacritty
	case l.Set("VSCODE_INJECTION"):
		return VSCode
	default:
		return Recognize(l.Identity())
	}
}

// Multiplexed reports a tmux, screen or Zellij session. A multiplexer re-encodes
// what the outer terminal sends, so the outer terminal's identity no longer
// describes the byte stream this program receives.
func (l Lookup) Multiplexed() bool {
	if l.Set("TMUX") || l.Set("STY") || l.Set("ZELLIJ") || l.Set("ZELLIJ_SESSION_NAME") {
		return true
	}
	term := l.Term()
	return l.Is("TERM_PROGRAM", "tmux") || strings.HasPrefix(term, "screen") || strings.HasPrefix(term, "tmux")
}

// WSL reports a Windows Subsystem for Linux session, whose terminal is bridged
// from Windows rather than attached to this kernel.
func (l Lookup) WSL() bool { return l.Set("WSL_INTEROP") || l.Set("WSL_DISTRO_NAME") }

// Memoized reads each variable from l at most once. A caller that must settle
// every environment fact before acquiring a terminal can then hand one stable
// view to detectors that read overlapping names.
func (l Lookup) Memoized() Lookup {
	if l == nil {
		return nil
	}
	type fact struct {
		value string
		set   bool
	}
	facts := make(map[string]fact)
	return func(name string) (string, bool) {
		f, ok := facts[name]
		if !ok {
			f.value, f.set = l(name)
			facts[name] = f
		}
		return f.value, f.set
	}
}

// Terminal is an emulator whose behaviour differs here in ways no protocol
// reports: wheel batching, image protocol and keyboard defects are answered per
// emulator by the package that owns each policy.
type Terminal uint8

const (
	UnknownTerminal Terminal = iota
	AppleTerminal
	Alacritty
	Cursor
	Ghostty
	ITerm2
	Kitty
	Mintty
	Rio
	VSCode
	Warp
	WezTerm
	Windsurf
	Zed
)

// identifyingNames are the lowercase substrings by which each emulator names itself
// in its environment or in an XTVERSION reply. Order settles an identity that
// contains more than one name.
var identifyingNames = [...]struct {
	name     string
	terminal Terminal
}{
	{"iterm", ITerm2},
	{"wezterm", WezTerm},
	{"mintty", Mintty},
	{"vscode", VSCode},
	{"cursor", Cursor},
	{"windsurf", Windsurf},
	{"zed", Zed},
	{"apple_terminal", AppleTerminal},
	{"ghostty", Ghostty},
	{"kitty", Kitty},
	{"alacritty", Alacritty},
	{"warp", Warp},
	{"rio", Rio},
}

// Recognize reads which emulator a name describes: one a terminal reported for
// itself, or an environment's [Lookup.Identity].
func Recognize(name string) Terminal {
	name = strings.ToLower(name)
	for _, known := range identifyingNames {
		if strings.Contains(name, known.name) {
			return known.terminal
		}
	}
	return UnknownTerminal
}

var terminalNames = [...]string{
	UnknownTerminal: "unknown", AppleTerminal: "apple_terminal", Alacritty: "alacritty", Cursor: "cursor",
	Ghostty: "ghostty", ITerm2: "iterm2", Kitty: "kitty", Mintty: "mintty", Rio: "rio", VSCode: "vscode",
	Warp: "warp", WezTerm: "wezterm", Windsurf: "windsurf", Zed: "zed",
}

func (t Terminal) String() string { return enum.Name(terminalNames[:], t, "Terminal") }
