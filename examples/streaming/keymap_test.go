package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"testing"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/input"
)

func TestCurrentPromptKeysOwnHintsAndStreamingActions(t *testing.T) {
	h := newHost(t)
	c, stop := runSource(t, h, func(ctx context.Context, _ string, dst io.Writer) error {
		if _, err := io.WriteString(dst, "stream running\n"); err != nil {
			return err
		}
		<-ctx.Done()
		return context.Cause(ctx)
	})
	defer stop()
	owner := c.runtime.Dispatcher()
	keys := base.DefaultEditorKeys()
	keys.Bind(send, input.Ctrl.Rune('s'))
	keys.Bind(cancelSource, input.Ctrl.Rune('k'))
	keys.Bind(quit, input.Ctrl.Rune('q'))
	if err := owner.Invoke(t.Context(), func() error { c.composer.Controller().SetKeys(keys); return nil }); err != nil {
		t.Fatal(err)
	}
	awaitText(t, h, "ctrl+s send", "ctrl+k cancel source")
	if err := owner.Invoke(t.Context(), func() error {
		c.composer.Controller().SetText("draft")
		for _, key := range []input.Key{
			{Code: input.Enter},
			{Code: input.Character, Rune: 'x', Mods: input.Ctrl},
			{Code: input.Character, Rune: 'c', Mods: input.Ctrl},
			{Code: input.Character, Rune: 's', Mods: input.Ctrl, Transition: input.Release},
		} {
			if c.Handle(key) {
				return fmt.Errorf("obsolete or released key was consumed: %+v", key)
			}
		}
		if !c.Handle(input.Key{Code: input.Character, Rune: 's', Mods: input.Ctrl}) || c.pending != "draft" || !c.dialog.Controller().Open() {
			return fmt.Errorf("rebound send did not request approval: pending=%q", c.pending)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	awaitText(t, h, "Approve prompt")
	h.events <- input.Key{Code: input.Enter}
	awaitText(t, h, "stream running")
	if err := owner.Invoke(t.Context(), func() error {
		if c.Handle(input.Key{Code: input.Character, Rune: 'x', Mods: input.Ctrl}) {
			return errors.New("obsolete cancel binding still controls the source")
		}
		if !c.Handle(input.Key{Code: input.Character, Rune: 'k', Mods: input.Ctrl}) {
			return errors.New("rebound cancel was ignored")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	awaitState(t, c, func(s state) bool { return !s.active && s.status == "cancelled" })
	if err := owner.Invoke(t.Context(), func() error {
		c.composer.Controller().SetKeys(nil)
		for _, key := range []input.Key{
			{Code: input.Enter},
			{Code: input.Character, Rune: 's', Mods: input.Ctrl},
			{Code: input.Character, Rune: 'k', Mods: input.Ctrl},
			{Code: input.Character, Rune: 'q', Mods: input.Ctrl},
		} {
			if c.Handle(key) {
				return fmt.Errorf("nil keys retained an application binding: %+v", key)
			}
		}
		c.composer.Controller().SetKeys(keys)
		if !c.quitKey(input.Key{Code: input.Character, Rune: 'q', Mods: input.Ctrl}) {
			return errors.New("rebound quit was ignored")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
