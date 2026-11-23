package termenv_test

import (
	"testing"

	"github.com/Tangerg/oolong/ui/core/termenv"
)

func environment(values map[string]string) termenv.Lookup {
	return func(name string) (string, bool) {
		value, ok := values[name]
		return value, ok
	}
}

func TestANilEnvironmentMakesNoClaims(t *testing.T) {
	var env termenv.Lookup
	if env.Value("TERM") != "" || env.Set("TERM") || env.Terminal() != termenv.UnknownTerminal ||
		env.Multiplexed() || env.WSL() || env.Memoized() != nil {
		t.Fatal("an absent environment named a terminal")
	}
}

func TestAnExportedEmptyVariableIsNotSet(t *testing.T) {
	env := environment(map[string]string{"NO_COLOR": "", "KITTY_WINDOW_ID": ""})
	if env.Set("NO_COLOR") || env.Terminal() == termenv.Kitty {
		t.Fatal("an empty value was read as a claim")
	}
	if !environment(map[string]string{"NO_COLOR": " "}).Set("NO_COLOR") {
		t.Fatal("a non-empty value was not read as a claim")
	}
}

func TestIdentityJoinsEveryNameATerminalUsesInLowerCase(t *testing.T) {
	env := environment(map[string]string{
		"TERM": "xterm-256color", "TERM_PROGRAM": "iTerm.app", "LC_TERMINAL": "iTerm2",
	})
	if got, want := env.Identity(), "xterm-256color iterm.app iterm2"; got != want {
		t.Fatalf("identity = %q, want %q", got, want)
	}
	if got, want := env.Term(), "xterm-256color"; got != want {
		t.Fatalf("term = %q, want %q", got, want)
	}
	if !env.Is("TERM_PROGRAM", "ITERM.APP") {
		t.Fatal("a name differing only in case did not match")
	}
}

func TestNativeMarkersOutrankNames(t *testing.T) {
	for _, test := range []struct {
		variable string
		want     termenv.Terminal
	}{
		{"KITTY_WINDOW_ID", termenv.Kitty},
		{"GHOSTTY_RESOURCES_DIR", termenv.Ghostty},
		{"ALACRITTY_SOCKET", termenv.Alacritty},
		{"VSCODE_INJECTION", termenv.VSCode},
	} {
		env := environment(map[string]string{test.variable: "1", "TERM_PROGRAM": "WezTerm"})
		if got := env.Terminal(); got != test.want {
			t.Errorf("%s identified %d, want %d", test.variable, got, test.want)
		}
	}
	if got := environment(map[string]string{"TERM_PROGRAM": "WezTerm"}).Terminal(); got != termenv.WezTerm {
		t.Fatalf("a terminal without a marker was identified as %d", got)
	}
}

func TestRecognizeReadsReportedAndEnvironmentNames(t *testing.T) {
	for name, want := range map[string]termenv.Terminal{
		"kitty(0.32.2)":                  termenv.Kitty,
		"xterm-ghostty":                  termenv.Ghostty,
		"iTerm2 3.5.0":                   termenv.ITerm2,
		"xterm-256color apple_terminal ": termenv.AppleTerminal,
		"tmux 3.4":                       termenv.UnknownTerminal,
		"":                               termenv.UnknownTerminal,
	} {
		if got := termenv.Recognize(name); got != want {
			t.Errorf("Recognize(%q) = %d, want %d", name, got, want)
		}
	}
}

func TestMultiplexersAreRecognizedByTheirSessionOrTerminalName(t *testing.T) {
	for _, values := range []map[string]string{
		{"TMUX": "/tmp/tmux-501/default,1,0"},
		{"STY": "1234.pts-0"},
		{"ZELLIJ": "0"},
		{"ZELLIJ_SESSION_NAME": "work"},
		{"TERM_PROGRAM": "tmux"},
		{"TERM": "screen-256color"},
		{"TERM": "tmux-256color"},
	} {
		if !environment(values).Multiplexed() {
			t.Errorf("%v was not recognized as multiplexed", values)
		}
	}
	if environment(map[string]string{"TERM": "xterm-256color"}).Multiplexed() {
		t.Fatal("a direct terminal was read as multiplexed")
	}
}

func TestWSLIsRecognizedByEitherBridgeVariable(t *testing.T) {
	for _, name := range []string{"WSL_INTEROP", "WSL_DISTRO_NAME"} {
		if !environment(map[string]string{name: "1"}).WSL() {
			t.Errorf("%s was not read as WSL", name)
		}
	}
}

func TestMemoizedReadsEachVariableOnce(t *testing.T) {
	reads := map[string]int{}
	env := termenv.Lookup(func(name string) (string, bool) {
		reads[name]++
		return "xterm", name == "TERM"
	}).Memoized()
	for range 3 {
		if value, ok := env("TERM"); value != "xterm" || !ok {
			t.Fatalf("memoized TERM = %q, %t", value, ok)
		}
		if _, ok := env("COLORTERM"); ok {
			t.Fatal("an unset variable became set")
		}
	}
	if reads["TERM"] != 1 || reads["COLORTERM"] != 1 {
		t.Fatalf("reads = %v, want one per name", reads)
	}
}
