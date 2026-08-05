package term_test

import (
	"os"
	"testing"

	"github.com/Tangerg/oolong/core/graphics"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/term"
)

// Detection is the one thing in this library that reads its environment rather
// than asking the terminal and letting the request be ignored. A truecolor
// sequence a terminal cannot read does not degrade, it prints wrong, and there is
// no request that fails safely — so what it reads is worth stating exactly.

func TestDetectDepth(t *testing.T) {
	for _, tc := range []struct {
		name string
		env  map[string]string
		want grid.Depth
	}{
		{"nothing said", map[string]string{}, grid.NoColor},
		{"NO_COLOR wins outright", map[string]string{
			"NO_COLOR": "", "COLORTERM": "truecolor", "TERM": "xterm-256color",
		}, grid.NoColor},
		{"NO_COLOR with a value is still NO_COLOR", map[string]string{"NO_COLOR": "0"}, grid.NoColor},
		{"COLORTERM says truecolor", map[string]string{"COLORTERM": "truecolor"}, grid.TrueColor},
		{"COLORTERM says 24bit", map[string]string{"COLORTERM": "24bit"}, grid.TrueColor},
		{"COLORTERM in capitals", map[string]string{"COLORTERM": "TrueColor"}, grid.TrueColor},
		{"a dumb terminal", map[string]string{"TERM": "dumb"}, grid.NoColor},
		{"a 256-colour TERM", map[string]string{"TERM": "xterm-256color"}, grid.Depth256},
		{"tmux with 256", map[string]string{"TERM": "tmux-256color"}, grid.Depth256},
		// The decision worth stating: plenty of terminals handle 24-bit colour and
		// call themselves plain xterm, so treating what we do not recognise as
		// sixteen colours would make the common case worse to fix the rare one.
		{"an unrecognised TERM", map[string]string{"TERM": "xterm"}, grid.TrueColor},
		{"something invented", map[string]string{"TERM": "myterm"}, grid.TrueColor},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("NO_COLOR", "")
			for _, k := range []string{"NO_COLOR", "COLORTERM", "TERM"} {
				if v, ok := tc.env[k]; ok {
					t.Setenv(k, v)
				} else {
					unset(t, k)
				}
			}
			if got := term.DetectDepth(); got != tc.want {
				t.Fatalf("= %v, want %v", got, tc.want)
			}
		})
	}
}

func TestDetectGraphics(t *testing.T) {
	for _, tc := range []struct {
		name string
		env  map[string]string
		want graphics.Protocol
	}{
		{"nothing said", map[string]string{}, graphics.None},
		{"kitty by window id", map[string]string{"KITTY_WINDOW_ID": "1"}, graphics.Kitty},
		{"ghostty by resources", map[string]string{"GHOSTTY_RESOURCES_DIR": "/opt"}, graphics.Kitty},
		{"kitty by TERM", map[string]string{"TERM": "xterm-kitty"}, graphics.Kitty},
		{"wezterm by program", map[string]string{"TERM_PROGRAM": "WezTerm"}, graphics.Kitty},
		{"iterm2 speaks its own", map[string]string{"TERM_PROGRAM": "iTerm.app"}, graphics.None},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, k := range []string{"KITTY_WINDOW_ID", "GHOSTTY_RESOURCES_DIR", "TERM", "TERM_PROGRAM"} {
				if v, ok := tc.env[k]; ok {
					t.Setenv(k, v)
				} else {
					unset(t, k)
				}
			}
			if got := term.DetectGraphics(); got != tc.want {
				t.Fatalf("= %v, want %v", got, tc.want)
			}
		})
	}
}

// unset clears a variable for the test and puts it back afterwards, which
// t.Setenv does not offer on its own.
func unset(t *testing.T, key string) {
	t.Helper()
	t.Setenv(key, "")
	if err := os.Unsetenv(key); err != nil {
		t.Fatal(err)
	}
}
