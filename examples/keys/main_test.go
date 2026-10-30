package main

import (
	"testing"

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

	host.Shows(t, "waiting for g or gg")
	host.Type("g")
	host.Shows(t, "resolved g")
	host.Type("gg")
	host.Shows(t, "resolved gg")
	host.Type("q")
	if err := <-done; err != nil {
		t.Fatalf("program: %v", err)
	}
}
