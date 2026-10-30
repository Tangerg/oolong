package main

import (
	"testing"

	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/program"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/uitest"
)

func TestTypingNarrowsAndEnterPicks(t *testing.T) {
	chosen := ""
	host := uitest.New(t, uitest.Config{Width: 60, Height: 12})
	done := make(chan error, 1)
	go func() {
		app, err := program.NewScreen(program.ScreenConfig{Host: host, Build: func(runtime *program.Runtime) (scene.Element, error) {
			return newPicker(runtime, files(), &chosen).Element(), nil
		}})
		if err == nil {
			err = app.Run(t.Context())
		}
		done <- err
	}()

	host.Shows(t, "type to narrow")
	host.Shows(t, "25 of 25")

	host.Type("termwri")
	host.Shows(t, "1 of 25")
	// "ter.go" and not "writer.go": the characters that answered the query are drawn
	// in another style, so the row reaches the terminal in pieces. That is the
	// feature, and a test that asserted on the whole word would be asserting that it
	// is not there.
	host.Shows(t, "ter.go")

	host.Press(input.Enter)
	if err := <-done; err != nil {
		t.Fatalf("the program ended with %v", err)
	}
	if chosen != "ui/core/term/writer.go" {
		t.Fatalf("picked %q", chosen)
	}
}
