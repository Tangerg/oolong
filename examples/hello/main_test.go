package main

import (
	"testing"

	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/program"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/uitest"
)

func TestHelpAndQuitFollowLiveKeyBindings(t *testing.T) {
	host := uitest.New(t, uitest.Config{Width: 60, Height: 8})
	ready := make(chan *hello, 1)
	done := make(chan error, 1)
	go func() {
		app, err := program.NewScreen(program.ScreenConfig{Host: host, Build: func(runtime *program.Runtime) (scene.Element, error) {
			h := newHello(runtime)
			ready <- h
			return h.Element(), nil
		}})
		if err == nil {
			err = app.Run(t.Context())
		}
		done <- err
	}()
	h := <-ready
	host.Shows(t, "q quit")
	if err := h.runtime.Dispatcher().Invoke(t.Context(), func() error {
		h.keys.Unbind(input.Chord{Rune: 'q'})
		h.keys.Unbind(input.Ctrl.Rune('c'))
		h.keys.Bind(quitHello, input.Alt.Rune('x'))
		h.element.Invalidate()
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	host.Shows(t, "alt+x quit")
	host.Type("q")
	host.Shows(t, "1 keys")
	host.Send(input.Key{Rune: 'x', Mods: input.Alt})
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestItDrawsAndAnswersAndStops(t *testing.T) {
	host := uitest.New(t, uitest.Config{Width: 60, Height: 12})
	done := make(chan error, 1)
	go func() {
		app, err := program.NewScreen(program.ScreenConfig{Host: host, Build: func(runtime *program.Runtime) (scene.Element, error) {
			return newHello(runtime).Element(), nil
		}})
		if err == nil {
			err = app.Run(t.Context())
		}
		done <- err
	}()

	host.Shows(t, "0 keys")
	host.Type("ab")
	host.Shows(t, "2 keys")

	host.Type("q")
	if err := <-done; err != nil {
		t.Fatalf("the program ended with %v", err)
	}
}
