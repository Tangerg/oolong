package main

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/program"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/uitest"
)

func TestCurrentPromptKeysOwnHintsAndSessionActions(t *testing.T) {
	host := uitest.New(t, uitest.Config{Width: 90, Height: 24})
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	ready := make(chan *agent, 1)
	finished := make(chan struct{})
	var runErr error
	go func() {
		defer close(finished)
		app, err := program.NewInline(program.InlineConfig{Host: host, Build: func(runtime *program.InlineRuntime) (scene.Element, error) { //nolint:contextcheck // Runtime work inherits Run's context.
			a, err := newAgent(runtime, mockBackend{})
			if err != nil {
				return scene.Element{}, err
			}
			ready <- a
			return a.Element(), nil
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
			t.Errorf("agent: %v", runErr)
		}
	})
	var a *agent
	select {
	case a = <-ready:
	case <-finished:
		t.Fatalf("agent did not start: %v", runErr)
	}
	host.Shows(t, "Ask the mock agent")
	keys := base.DefaultEditorKeys()
	keys.Bind(sendPrompt, input.Ctrl.Rune('s'))
	keys.Bind(cancelSource, input.Ctrl.Rune('k'))
	keys.Bind(quitAgent, input.Ctrl.Rune('q'))
	owner := a.runtime.Dispatcher()
	if err := owner.Invoke(ctx, func() error { a.composer.Controller().SetKeys(keys); return nil }); err != nil {
		t.Fatal(err)
	}
	for _, hint := range []string{"ctrl+s send", "ctrl+k cancel source", "ctrl+q quit"} {
		host.Shows(t, hint)
	}
	host.Type("draft")
	host.Press(input.Enter)
	host.Type("-kept")
	if _, err := host.Repaint(ctx); err != nil {
		t.Fatal(err)
	}
	if err := owner.Invoke(ctx, func() error {
		if a.run != nil || a.composer.Controller().Text() != "draft-kept" {
			return fmt.Errorf("obsolete Enter advanced the prompt: %q", a.composer.Controller().Text())
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	host.Send(input.Key{Code: input.Character, Rune: 's', Mods: input.Ctrl})
	host.Shows(t, "Review tool call")
	if err := owner.Invoke(ctx, func() error {
		for _, key := range []input.Key{
			{Code: input.Character, Rune: 'x', Mods: input.Ctrl},
			{Code: input.Character, Rune: 'c', Mods: input.Ctrl},
			{Code: input.Character, Rune: 'k', Mods: input.Ctrl, Transition: input.Release},
		} {
			if a.sessionKey(key) {
				return fmt.Errorf("obsolete or released session key was consumed: %+v", key)
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	host.Send(input.Key{Code: input.Character, Rune: 'k', Mods: input.Ctrl})
	host.Shows(t, "cancelled")
	if err := owner.Invoke(ctx, func() error {
		a.composer.Controller().SetKeys(nil)
		for _, key := range []input.Key{
			{Code: input.Enter},
			{Code: input.Character, Rune: 's', Mods: input.Ctrl},
			{Code: input.Character, Rune: 'k', Mods: input.Ctrl},
			{Code: input.Character, Rune: 'q', Mods: input.Ctrl},
		} {
			if a.Handle(key) {
				return fmt.Errorf("nil keys retained an application binding: %+v", key)
			}
		}
		a.composer.Controller().SetKeys(keys)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	host.Type("/he")
	host.Shows(t, "show the commands")
	host.Send(input.Key{Code: input.Character, Rune: 'q', Mods: input.Ctrl})
	select {
	case <-finished:
	case <-ctx.Done():
		t.Fatal("rebound quit did not end the agent with completion open")
	}
}
