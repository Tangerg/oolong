package headless

import (
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/keymap"
)

func TestBindingTransferIsCompleteBeforeCancellationCanReenter(t *testing.T) {
	for _, kind := range []string{"Text", "Select", "MultiSelect", "Confirm"} {
		t.Run(kind, func(t *testing.T) {
			var field Field
			var matcher *keymap.Matcher
			var current func() string
			var middle, last func()
			switch kind {
			case "Text":
				old, next, latest := "old", "middle", "last"
				f := NewText(TextConfig{Value: Bind(&old)})
				field, matcher, current = f, &f.editor.matcher, f.Text
				middle, last = func() { f.SetBinding(Bind(&next)) }, func() { f.SetBinding(Bind(&latest)) }
			case "Select":
				old, next, latest := "old", "middle", "last"
				f := NewSelect(SelectConfig[string]{Value: Bind(&old), Same: Equal[string]})
				f.SetOptions(Options("old", "middle", "last"))
				field, matcher, current = f, &f.matcher, func() string { choice, _ := f.Chosen(); return choice.Value }
				middle, last = func() { f.SetBinding(Bind(&next)) }, func() { f.SetBinding(Bind(&latest)) }
			case "MultiSelect":
				old, next, latest := []string{"old"}, []string{"middle"}, []string{"last"}
				f := NewMultiSelect(MultiSelectConfig[string]{Value: Bind(&old), Same: Equal[string]})
				f.SetOptions(Options("old", "middle", "last"))
				field, matcher, current = f, &f.matcher, func() string { return strings.Join(f.Taken(), ",") }
				middle, last = func() { f.SetBinding(Bind(&next)) }, func() { f.SetBinding(Bind(&latest)) }
			case "Confirm":
				old, next, latest := false, true, false
				f := NewConfirm(ConfirmConfig{Value: Bind(&old)})
				field, matcher, current = f, &f.matcher, func() string {
					if f.Answer() {
						return "middle"
					}
					return "last"
				}
				middle, last = func() { f.SetBinding(Bind(&next)) }, func() { f.SetBinding(Bind(&latest)) }
			}
			if err := field.Validate(); err != nil {
				t.Fatal(err)
			}
			validated := field.Validation()
			var keys keymap.Map
			keys.Bind(Toggle, input.Chord{Rune: 'g'})
			keys.Bind(SelectNext, input.Chord{Rune: 'g'}, input.Chord{Rune: 'g'})
			keys.Resolve = func(time.Duration, func()) func() {
				return func() {
					if current() != "middle" || field.Validation() != nil || validated() {
						t.Fatal("cancellation observed an incomplete owner transfer")
					}
					last()
				}
			}
			matcher.Handle(&keys, input.Key{Rune: 'g'}, func(keymap.Action) bool { return true })
			middle()
			if current() != "last" || field.Validation() != nil || validated() || len(matcher.Keys()) != 0 {
				t.Fatal("old binding transfer overwrote reentry or retained old validation and matching")
			}
		})
	}
}
