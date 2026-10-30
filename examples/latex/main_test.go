package main

import (
	"testing"

	"github.com/Tangerg/oolong/ui/core/program"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/uitest"
)

func TestTheFormulaDrawsDirectlyAndQuits(t *testing.T) {
	host := uitest.New(t, uitest.Config{Width: 72, Height: 16})
	done := make(chan error, 1)
	go func() {
		app, err := program.NewScreen(program.ScreenConfig{Host: host, Build: func(runtime *program.Runtime) (scene.Element, error) {
			return newLatex(runtime).Element(), nil
		}})
		if err == nil {
			err = app.Run(t.Context())
		}
		done <- err
	}()

	host.Shows(t, "LaTeX")
	host.Shows(t, "√")
	host.Shows(t, "±")
	host.Shows(t, "latex.Formula")

	host.Type("q")
	if err := <-done; err != nil {
		t.Fatalf("the program ended with %v", err)
	}
}
