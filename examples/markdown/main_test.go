package main

import (
	"testing"

	"github.com/Tangerg/oolong/ui/core/program"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/uitest"
)

func TestTheDocumentReflowsAndQuits(t *testing.T) {
	host := uitest.New(t, uitest.Config{Width: 72, Height: 20})
	done := make(chan error, 1)
	go func() {
		app, err := program.NewScreen(program.ScreenConfig{Host: host, Build: func(runtime *program.Runtime) (scene.Element, error) {
			return newMarkdown(runtime).Element(), nil
		}})
		if err == nil {
			err = app.Run(t.Context())
		}
		done <- err
	}()

	host.Shows(t, "Structured text stays structured")
	host.Shows(t, "immutable blocks")
	host.Shows(t, "final grid")
	host.Resize(44, 20)
	host.Shows(t, "Parsing, measuring, drawing")

	host.Type("q")
	if err := <-done; err != nil {
		t.Fatalf("the program ended with %v", err)
	}
}
