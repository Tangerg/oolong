package main

import (
	"testing"

	"github.com/Tangerg/oolong/ui/core/program"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/uitest"
)

func TestThePeerRenderersComposeAndQuit(t *testing.T) {
	host := uitest.New(t, uitest.Config{Width: 88, Height: 22})
	done := make(chan error, 1)
	go func() {
		app, err := program.NewScreen(program.ScreenConfig{Host: host, Build: func(runtime *program.Runtime) (scene.Element, error) {
			return newContent(runtime).Element(), nil
		}})
		if err == nil {
			err = app.Run(t.Context())
		}
		done <- err
	}()

	host.Shows(t, "Three peers, one document")
	host.Shows(t, "explicitly bind")
	host.Shows(t, "√")
	host.Shows(t, "drawable contract")
	host.Type("2")
	host.Shows(t, "println")
	host.Type("3")
	host.Shows(t, "√")
	host.Type("1")
	host.Shows(t, "Three peers, one document")

	host.Type("q")
	if err := <-done; err != nil {
		t.Fatalf("the program ended with %v", err)
	}
}
