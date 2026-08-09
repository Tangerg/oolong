package term_test

import (
	"strconv"
	"strings"
	"testing"

	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/term"
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
	if got := (term.Options{Keyboard: features}).Modes(nil).Enter(); !strings.Contains(got, "\x1b[>5u") {
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
			modes := (term.Options{Keyboard: input.KeyboardAll | 1<<20}).Modes(lookup)
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
