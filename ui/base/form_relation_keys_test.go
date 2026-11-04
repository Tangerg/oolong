package base_test

import (
	"testing"
	"time"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/keymap"
	"github.com/Tangerg/oolong/ui/core/scene"
)

func TestFormFieldReplacementCancelsItsPreviousKeyboardSequence(t *testing.T) {
	for _, sequence := range []string{"deferred", "partial"} {
		for _, relation := range []string{"retained focus", "new fields"} {
			t.Run(sequence+"/"+relation, func(t *testing.T) {
				keys, resolve := deferredKeys(base.Submit, base.Cancel)
				if sequence == "partial" {
					keys = &keymap.Map{}
					keys.Bind(base.Submit, input.Chord{Rune: 'x'}, input.Chord{Rune: 'x'})
				}
				first := base.NewConfirm(base.ConfirmConfig{Label: "original"})
				replacement := base.NewConfirm(base.ConfirmConfig{Label: "replacement"})
				submissions := 0
				form := base.NewForm(base.FormConfig{
					Fields: []base.Field{first}, Keys: keys,
					Submit: func(base.SubmissionID) (scene.ActionState, error) {
						submissions++
						return scene.Applied, nil
					},
				})
				fixture := newTestRoot(form)
				t.Cleanup(fixture.root.Close)
				focusElement(t, first.Element())
				if !fixture.root.Handle(input.Key{Rune: 'x'}) {
					t.Fatal("form did not retain the first key")
				}
				if relation == "retained focus" {
					form.Set(first, replacement)
				} else {
					form.Set(replacement)
				}
				if sequence == "deferred" {
					if *resolve == nil {
						t.Fatal("form did not defer its ambiguous key")
					}
					(*resolve)()
				} else {
					fixture.root.Handle(input.Key{Rune: 'x'})
				}
				if submissions != 0 {
					t.Fatal("a key sequence for the previous fields submitted their replacement")
				}
			})
		}
	}
}

func TestRejectedFormFieldReplacementKeepsItsKeyboardSequence(t *testing.T) {
	keys, resolve := deferredKeys(base.Submit, base.Cancel)
	field := base.NewConfirm(base.ConfirmConfig{})
	submissions := 0
	form := base.NewForm(base.FormConfig{
		Fields: []base.Field{field}, Keys: keys,
		Submit: func(base.SubmissionID) (scene.ActionState, error) {
			submissions++
			return scene.Applied, nil
		},
	})
	fixture := newTestRoot(form)
	t.Cleanup(fixture.root.Close)
	focusElement(t, field.Element())
	fixture.root.Handle(input.Key{Rune: 'x'})
	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("duplicate field relation was accepted")
			}
		}()
		form.Set(field, field)
	}()
	if *resolve == nil {
		t.Fatal("form did not defer its ambiguous key")
	}
	(*resolve)()
	if submissions != 1 {
		t.Fatal("a rejected relation replacement revoked the current sequence")
	}
}

func TestFormFieldPublicationPrecedesReentrantResolverCancellation(t *testing.T) {
	keys, _ := deferredKeys(base.Submit, base.Cancel)
	first := base.NewConfirm(base.ConfirmConfig{Label: "retained"})
	second := base.NewConfirm(base.ConfirmConfig{Label: "published"})
	latest := base.NewConfirm(base.ConfirmConfig{Label: "newer"})
	form := base.NewForm(base.FormConfig{Fields: []base.Field{first}, Keys: keys})
	cancellations := 0
	keys.Resolve = func(_ time.Duration, _ func()) func() {
		return func() {
			cancellations++
			fields := form.Fields()
			if len(fields) != 2 || fields[1] != second || !second.Element().Alive() {
				t.Fatal("resolver cancellation observed a partial field relation")
			}
			form.Set(first, latest)
		}
	}
	fixture := newTestRoot(form)
	t.Cleanup(fixture.root.Close)
	focusElement(t, first.Element())
	fixture.root.Handle(input.Key{Rune: 'x'})
	form.Set(first, second)
	fields := form.Fields()
	if cancellations != 1 {
		t.Fatal("field replacement did not cancel the pending resolver")
	}
	if len(fields) != 2 || fields[1] != latest || !latest.Element().Alive() {
		t.Fatal("the outer field replacement overwrote a newer relation from cancellation")
	}
}
