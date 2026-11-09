package main

import (
	"testing"

	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/program"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/uitest"
)

func TestPickerLeavesModifiedChordsWithTheEditor(t *testing.T) {
	for _, key := range []input.Key{
		{Code: input.Enter, Mods: input.Alt},
		{Code: input.Enter, Mods: input.Ctrl},
		{Code: input.Enter, Mods: input.Shift},
		{Code: input.KeypadEnter, Mods: input.Alt},
		{Code: input.Enter, Transition: input.Release},
		{Code: input.Esc, Mods: input.Alt},
		{Code: input.Up, Mods: input.Shift},
		{Code: input.Down, Mods: input.Shift},
		{Code: input.PageDown, Mods: input.Ctrl},
	} {
		t.Run(key.String(), func(t *testing.T) {
			host := uitest.New(t, uitest.Config{Width: 60, Height: 12})
			chosen := ""
			app, err := program.NewScreen(program.ScreenConfig{Host: host, Build: func(runtime *program.Runtime) (scene.Element, error) {
				p := newPicker(runtime, []string{"first", "second"}, &chosen)
				if consumed := p.Handle(key); consumed || chosen != "" {
					t.Errorf("picker consumed editor input %+v: chosen=%q", key, chosen)
				}
				if item, _ := p.list.Current(); item != "first" {
					t.Errorf("modified navigation moved the list to %q", item)
				}
				runtime.Quit()
				return p.Element(), nil
			}})
			if err != nil {
				t.Fatal(err)
			}
			if err := app.Run(t.Context()); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestPickerUsesCanonicalChordsForNavigationAndSelection(t *testing.T) {
	host := uitest.New(t, uitest.Config{Width: 60, Height: 12})
	chosen := ""
	app, err := program.NewScreen(program.ScreenConfig{Host: host, Build: func(runtime *program.Runtime) (scene.Element, error) {
		p := newPicker(runtime, []string{"first", "second"}, &chosen)
		if !p.Handle(input.Key{Code: input.Down}) {
			t.Error("plain Down did not navigate")
		}
		if !p.Handle(input.Key{Code: input.KeypadEnter}) || chosen != "second" {
			t.Errorf("keypad Enter did not select the current result: %q", chosen)
		}
		return p.Element(), nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := app.Run(t.Context()); err != nil {
		t.Fatal(err)
	}
}
