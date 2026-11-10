package term_test

import (
	"strconv"
	"strings"
	"testing"

	"github.com/Tangerg/oolong/ui/core/grid"

	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/term"
)

func TestCompatibleKeyboardRequestsOnlyPortableFeatures(t *testing.T) {
	features := term.KeyboardCompatible
	if !features.Has(input.KeyboardDisambiguate) ||
		!features.Has(input.KeyboardReportAlternates) {
		t.Fatalf("compatible keyboard features = %d", features)
	}
	for _, feature := range []input.KeyboardFeatures{
		input.KeyboardReportEvents,
		input.KeyboardReportAllAsEscapes,
		input.KeyboardReportText,
	} {
		if features.Has(feature) {
			t.Errorf("compatible keyboard unexpectedly requests feature %d", feature)
		}
	}
	if got := mustModes(t, term.Config{Features: term.Features{Keyboard: features}}, nil).Enter(); !strings.Contains(got, "\x1b[>5u") {
		t.Fatalf("compatible keyboard mode = %q, want flags 5", got)
	}
}

func TestKeyboardModesUseTheDrivenTerminalEnvironment(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want input.KeyboardFeatures
	}{
		{
			name: "ordinary terminal",
			want: input.KeyboardAll,
		},
		{
			name: "iTerm drops event reports",
			env:  map[string]string{"TERM_PROGRAM": "iTerm.app"},
			want: input.KeyboardAll &^ input.KeyboardReportEvents,
		},
		{
			name: "VS Code through WSL disables the protocol",
			env: map[string]string{
				"WSL_DISTRO_NAME": "Ubuntu",
				"TERM_PROGRAM":    "vscode",
			},
		},
		{
			name: "VS Code injection through WSL disables the protocol",
			env: map[string]string{
				"WSL_INTEROP":      "/run/WSL/1_interop",
				"VSCODE_INJECTION": "1",
			},
		},
		{
			name: "VS Code outside WSL keeps the protocol",
			env:  map[string]string{"TERM_PROGRAM": "vscode"},
			want: input.KeyboardAll,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lookup := func(name string) (string, bool) {
				value, ok := tt.env[name]
				return value, ok
			}
			modes := mustModes(t, term.Config{Features: term.Features{Keyboard: input.KeyboardAll | 1<<20}}, lookup)
			enter, leave := modes.Enter(), modes.Leave()
			if tt.want == 0 {
				if strings.Contains(enter, "\x1b[>") || strings.Contains(leave, "\x1b[<u") {
					t.Fatalf("filtered keyboard mode was still acquired: enter %q leave %q", enter, leave)
				}
				return
			}
			want := "\x1b[>" + strconv.Itoa(int(tt.want)) + "u"
			if !strings.Contains(enter, want) {
				t.Fatalf("keyboard mode = %q, want %q", enter, want)
			}
			if !strings.Contains(leave, "\x1b[<u") {
				t.Fatalf("keyboard mode was not restored: %q", leave)
			}
		})
	}
}

func TestKeyboardTextRequestsItsWirePrerequisite(t *testing.T) {
	modes := mustModes(t, term.Config{Features: term.Features{Keyboard: input.KeyboardReportText}}, nil)
	if !strings.Contains(modes.Enter(), "\x1b[>24u") || !strings.Contains(modes.Leave(), "\x1b[<u") {
		t.Fatalf("text-reporting acquisition %q, restoration %q", modes.Enter(), modes.Leave())
	}
}

func TestAcquisitionOwnsOneRequestedKeyboardObservation(t *testing.T) {
	for _, features := range []term.Features{
		{}, {Probe: true}, {Keyboard: term.KeyboardCompatible}, {Probe: true, Keyboard: term.KeyboardCompatible},
	} {
		modes := mustModes(t, term.Config{Features: features}, nil)
		want := 0
		if features.Probe || features.Keyboard != 0 {
			want = 1
		}
		for range 2 {
			if got := strings.Count(modes.Enter(), "\x1b[?u"); got != want {
				t.Fatalf("keyboard observation count = %d, want %d", got, want)
			}
		}
		if strings.Contains(modes.Leave(), "\x1b[?u") {
			t.Fatal("restoration queried a terminal whose input is being released")
		}
	}
}

func mustModes(t *testing.T, cfg term.Config, lookup func(string) (string, bool)) term.Modes {
	t.Helper()
	modes, err := cfg.Modes(lookup)
	if err != nil {
		t.Fatal(err)
	}
	return modes
}

func TestModesRefuseAnUnrestorableCursor(t *testing.T) {
	if _, err := (term.Config{RestoreCursor: grid.CursorStyle{Shape: grid.CursorShape(255)}}).Modes(nil); err == nil {
		t.Fatal("an invalid restoration target produced modes")
	}
}
