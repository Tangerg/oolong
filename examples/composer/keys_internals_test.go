package main

import (
	"context"
	"testing"
	"time"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/keymap"
	"github.com/Tangerg/oolong/ui/core/program"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/uitest"
)

func TestModifiedEnterCannotSubmitThePrompt(t *testing.T) {
	for _, key := range []input.Key{
		{Code: input.Enter, Mods: input.Alt},
		{Code: input.Enter, Mods: input.Ctrl},
		{Code: input.Enter, Mods: input.Shift},
		{Code: input.Enter, Transition: input.Release},
	} {
		p := newPrompt(nil)
		editor := p.composer.Controller()
		editor.SetText("draft")
		if p.Handle(key) {
			t.Fatalf("prompt consumed an editor key: %+v", key)
		}
		editor.Handle(key)
		if editor.Text() != "draft" || p.history.Len() != 0 {
			t.Fatalf("key %+v submitted or changed the draft: %q", key, editor.Text())
		}
	}
}

func TestPromptActionsUseTheCurrentEditorKeyMap(t *testing.T) {
	p := newPrompt(nil)
	p.history.Add(draft{text: "earlier"})
	editor := p.composer.Controller()
	editor.SetText("draft")
	keys := &keymap.Map{}
	keys.Bind(submitPrompt, input.Ctrl.Rune('s'))
	keys.Bind(previousPrompt, input.Ctrl.Rune('p'))
	keys.Bind(nextPrompt, input.Ctrl.Rune('n'))
	keys.Bind(quitPrompt, input.Ctrl.Rune('q'))
	editor.SetKeys(keys)
	for _, obsolete := range []input.Key{
		{Code: input.Enter},
		{Code: input.Up},
		{Code: input.Down},
		{Code: input.Character, Rune: 'c', Mods: input.Ctrl},
	} {
		if p.Handle(obsolete) || editor.Text() != "draft" || p.history.Walking() {
			t.Fatalf("obsolete binding %+v still advanced prompt state", obsolete)
		}
	}
	for _, step := range []struct {
		key  rune
		want string
	}{
		{'p', "earlier"}, {'n', "draft"}, {'s', ""}, {'q', ""},
	} {
		if !p.Handle(input.Key{Code: input.Character, Rune: step.key, Mods: input.Ctrl}) {
			t.Fatalf("rebound ctrl+%c was ignored", step.key)
		}
		if got := editor.Text(); got != step.want {
			t.Fatalf("ctrl+%c: draft %q, want %q", step.key, got, step.want)
		}
	}
	if sent, ok := p.history.At(1); !ok || sent.text != "draft" {
		t.Fatalf("rebound submit did not record the draft: %+v", sent)
	}
}

func TestCompletionPrecedesPromptNavigationBindings(t *testing.T) {
	p := newPrompt(nil)
	p.history.Add(draft{text: "earlier"})
	editor := p.composer.Controller()
	editor.Keys().Bind(previousPrompt, input.Chord{Code: input.Tab})
	editor.SetText("@rea")
	if !p.completion.Open() {
		t.Fatal("reference completion did not open")
	}
	if !p.Handle(input.Key{Code: input.Tab}) {
		t.Fatal("completion did not consume its accept key")
	}
	if editor.Text() != "@README.md" || p.history.Walking() {
		t.Fatalf("history overrode the active completion: %q", editor.Text())
	}
}

func TestReboundPromptHintsAndActionsAgreeInTheScene(t *testing.T) {
	host := uitest.New(t, uitest.Config{Width: 80, Height: 16})
	ctx, cancel := context.WithCancel(t.Context())
	finished := make(chan struct{})
	var runErr error
	go func() {
		defer close(finished)
		app, err := program.NewScreen(program.ScreenConfig{Host: host, Build: func(runtime *program.Runtime) (scene.Element, error) {
			p := newPrompt(runtime)
			keys := base.DefaultEditorKeys()
			keys.Bind(submitPrompt, input.Ctrl.Rune('s'))
			keys.Bind(previousPrompt, input.Ctrl.Rune('p'))
			keys.Bind(nextPrompt, input.Ctrl.Rune('n'))
			keys.Bind(quitPrompt, input.Ctrl.Rune('q'))
			p.composer.Controller().SetKeys(keys)
			return p.Element(), nil
		}})
		if err == nil {
			err = app.Run(ctx)
		}
		runErr = err
	}()
	t.Cleanup(func() {
		cancel()
		<-finished
		if runErr != nil {
			t.Errorf("composer stopped with %v", runErr)
		}
	})
	for _, hint := range []string{"ctrl+s submit", "ctrl+p previous prompt", "ctrl+n next prompt", "ctrl+q quit"} {
		host.Shows(t, hint)
	}
	host.Type("alpha")
	host.Send(input.Key{Code: input.Enter, Mods: input.Alt})
	host.Press(input.Enter)
	host.Type("-kept")
	host.Send(input.Key{Code: input.Character, Rune: 's', Mods: input.Ctrl})
	host.Shows(t, "sent: alpha-kept")
	host.Type("draft")
	host.Send(input.Key{Code: input.Character, Rune: 'p', Mods: input.Ctrl})
	host.Type("-again")
	host.Send(input.Key{Code: input.Character, Rune: 's', Mods: input.Ctrl})
	host.Shows(t, "sent: alpha-kept-again")
	host.Type("unsent")
	host.Send(input.Key{Code: input.Character, Rune: 'p', Mods: input.Ctrl})
	host.Send(input.Key{Code: input.Character, Rune: 'n', Mods: input.Ctrl})
	host.Send(input.Key{Code: input.Character, Rune: 's', Mods: input.Ctrl})
	host.Shows(t, "sent: unsent")
	host.Type("@rea")
	host.Shows(t, "project introduction")
	host.Send(input.Key{Code: input.Character, Rune: 'q', Mods: input.Ctrl})
	select {
	case <-finished:
	case <-time.After(5 * time.Second):
		t.Fatal("rebound quit did not take priority over the active completion")
	}
}
