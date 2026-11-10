package base_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/keymap"
	"github.com/Tangerg/oolong/ui/core/scene"
)

type invalidConfirmControl struct{ failure error }

func (*invalidConfirmControl) Snapshot() base.ConfirmSnapshot { return base.ConfirmSnapshot{} }
func (*invalidConfirmControl) Subscribe(func()) func()        { return func() {} }
func (c *invalidConfirmControl) Change(context.Context, base.ConfirmRequest) (scene.ActionResult[base.ConfirmSnapshot], error) {
	return scene.ActionResult[base.ConfirmSnapshot]{State: scene.Started + 1}, c.failure
}

func TestConfirmRejectsUnknownOutcomesAcrossRequestPaths(t *testing.T) {
	problem := errors.New("confirmation owner failed")
	field := base.NewConfirm(base.ConfirmConfig{Control: &invalidConfirmControl{failure: problem}})
	t.Cleanup(field.Close)
	root := semanticOwner(t, field.Element())
	result, err := field.Request(t.Context(), base.ConfirmRequest{Value: true})
	assertRejectedOutcome(t, result.State, err, problem)
	if !root.Handle(input.Key{Rune: ' '}) {
		t.Fatal("confirmation input was not handled")
	}
	assertRejectedOutcome(t, scene.Unchanged, field.ChangeError(), problem)
	invoked, err := root.Invoke(t.Context(), field.Element().Target(), scene.ActionSetValue, true)
	assertRejectedOutcome(t, invoked.State, err, problem)
	if field.Answer() {
		t.Fatal("controller invented an accepted answer")
	}
}

func TestSettingsRejectsUnknownOutcomesAcrossRequestPaths(t *testing.T) {
	problem := errors.New("settings owner failed")
	settings := base.NewSettings(base.SettingsConfig[string]{
		Change: func(context.Context, int, string, keymap.Action) (scene.ActionResult[bool], error) {
			return scene.ActionResult[bool]{State: scene.Started + 1, Value: true}, problem
		},
	})
	settings.List().SetItems([]string{"volume"})
	root := semanticOwner(t, settings.Element())
	result, err := settings.Request(t.Context(), 0, base.Increase)
	assertRejectedOutcome(t, result.State, err, problem)
	if result.Value || root.Handle(input.Key{Code: input.Right}) {
		t.Fatal("invalid result reported accepted input")
	}
	assertRejectedOutcome(t, scene.Unchanged, settings.ChangeError(), problem)
	address := base.CollectionIndex{ContentRevision: semanticRevision(t, root, settings.Element())}
	invoked, err := root.Invoke(t.Context(), settings.Element().Target(), scene.ActionIncrement, address)
	assertRejectedOutcome(t, invoked.State, err, problem)
}

func TestFormUsesTheSceneOutcomeAdmissionRule(t *testing.T) {
	problem := errors.New("submission owner failed")
	form := base.NewForm(base.FormConfig{Submit: func(base.SubmissionID) (scene.ActionState, error) {
		return scene.Started + 1, problem
	}})
	result, err := form.Submit()
	assertRejectedOutcome(t, result.State, err, problem)
	assertRejectedOutcome(t, scene.Unchanged, form.Error(), problem)
	if form.Submission().State != base.SubmissionFailed {
		t.Fatalf("invalid callback left submission in state %v", form.Submission().State)
	}
}

func assertRejectedOutcome(t *testing.T, state scene.ActionState, err, problem error) {
	t.Helper()
	if state != scene.Unchanged || !errors.Is(err, scene.ErrInvalid) || !errors.Is(err, problem) {
		t.Fatalf("invalid owner outcome escaped its boundary: state=%d, error=%v", state, err)
	}
}
