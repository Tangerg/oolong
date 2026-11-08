package main

import (
	"testing"
	"testing/synctest"

	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/program"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/uitest"
)

func TestQuitUsesCanonicalShortcuts(t *testing.T) {
	for _, test := range []struct {
		name     string
		key      input.Key
		wantQuit bool
	}{
		{"ctrl-c", input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl}, true},
		{"escape", input.Key{Code: input.Esc}, true},
		{"unmodified", input.Key{Code: input.Character, Rune: 'c'}, false},
		{"release", input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl, Transition: input.Release}, false},
		{"extra-alt", input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl | input.Alt}, false},
		{"alternate", input.Key{Code: input.Character, Rune: 'с', BaseRune: 'c', Mods: input.Ctrl}, true},
		{"extra-shift", input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl | input.Shift}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				form := ask(new(answers))
				host := uitest.New(t, uitest.Config{Width: 50, Height: 20})
				done := make(chan error, 1)
				go func() {
					app, err := program.NewScreen(program.ScreenConfig{Host: host, Build: func(runtime *program.Runtime) (scene.Element, error) { return dress(runtime, form), nil }})
					if err == nil {
						err = app.Run(t.Context())
					}
					done <- err
				}()
				synctest.Wait()
				host.Send(test.key)
				synctest.Wait()
				quit := false
				select {
				case err := <-done:
					quit = true
					if err != nil {
						t.Error(err)
					}
				default:
				}
				if quit != test.wantQuit {
					t.Errorf("quit=%v, want %v for key %+v", quit, test.wantQuit, test.key)
				}
				if !quit {
					host.Press(input.Esc)
					synctest.Wait()
					if err := <-done; err != nil {
						t.Error(err)
					}
				}
			})
		})
	}
}
