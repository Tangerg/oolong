package main

import (
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/keymap"
	"github.com/Tangerg/oolong/ui/core/program"
	"github.com/Tangerg/oolong/ui/uitest"
)

func TestExactAndLongerSequencesBothWork(t *testing.T) {
	host := uitest.New(t, uitest.Config{Width: 70, Height: 12})
	done := make(chan error, 1)
	go func() {
		app, err := program.NewScreen(program.ScreenConfig{Host: host, Build: newKeys})
		if err == nil {
			err = app.Run(t.Context())
		}
		done <- err
	}()

	host.Shows(t, "waiting for a shortcut")
	host.Type("g")
	host.Shows(t, "resolved next")
	host.Type("gg")
	host.Shows(t, "resolved top")
	host.Type("q")
	if err := <-done; err != nil {
		t.Fatalf("program: %v", err)
	}
}

func TestHelpReadsTheCurrentBindingsAndTimeout(t *testing.T) {
	bindings := new(keymap.Map)
	bindings.SetTimeout(750 * time.Millisecond)
	bindings.Bind(next, input.Chord{Rune: 'j'})
	bindings.Bind(top, input.Chord{Rune: 'j'}, input.Chord{Rune: 'j'})
	bindings.Bind(quit, input.Chord{Rune: 'x'})
	k := &keys{mapKeys: bindings}
	surface := grid.NewSurface(70, 12)
	k.Draw(surface.View())
	visible := strings.Join(surface.Rows(), "\n")
	//nolint:dupword // "j j" denotes two consecutive presses in the bound sequence.
	for _, want := range []string{"j next", "j j top", "x quit", "prefix timeout: 750ms"} {
		if !strings.Contains(visible, want) {
			t.Errorf("help does not show %q: %s", want, visible)
		}
	}
}
