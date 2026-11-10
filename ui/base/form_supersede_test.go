package base_test

import (
	"errors"
	"runtime"
	"slices"
	"testing"
	"time"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/scene"
)

// A newer submission revokes the pending one, which Settle can then no longer
// reach. Its owner used to be told nothing, so its work kept running unseen.
func TestASupersededSubmissionIsCancelled(t *testing.T) {
	var started, cancelled []base.SubmissionID
	form := base.NewForm(base.FormConfig{
		Fields: []base.Field{base.NewText(base.TextConfig{})},
		Submit: func(id base.SubmissionID) (scene.ActionState, error) {
			started = append(started, id)
			return scene.Started, nil
		},
		Cancel: func(id base.SubmissionID) error {
			cancelled = append(cancelled, id)
			return nil
		},
	})
	semanticOwner(t, form.Element())
	if _, err := form.Submit(); err != nil {
		t.Fatal(err)
	}
	if _, err := form.Submit(); err != nil {
		t.Fatal(err)
	}
	if len(started) != 2 || !slices.Equal(cancelled, started[:1]) {
		t.Fatalf("started %v, cancelled %v: want the first attempt cancelled", started, cancelled)
	}
	form.Set(base.NewText(base.TextConfig{}))
	if !slices.Equal(cancelled, started) {
		t.Fatalf("replacing the fields left the second attempt running: cancelled %v", cancelled)
	}
}

func TestSupersededFormWorkIsCancelledWhenPublicationExitsAbnormally(t *testing.T) {
	for _, operation := range []string{"submit", "replace fields", "resolver cancellation", "cancellation callback"} {
		for _, exit := range []string{"panic", "Goexit"} {
			t.Run(operation+"/"+exit, func(t *testing.T) {
				interrupt := false
				abort := func() {
					if !interrupt {
						return
					}
					if exit == "Goexit" {
						runtime.Goexit()
					}
					panic("interrupted callback")
				}
				keys, _ := deferredKeys(base.Submit, base.Cancel)
				if operation == "resolver cancellation" {
					keys.SetResolver(func(time.Duration, func()) func() { return abort })
				}
				var cancelled []base.SubmissionID
				form := base.NewForm(base.FormConfig{
					Fields: []base.Field{base.NewConfirm(base.ConfirmConfig{})}, Keys: keys,
					Submit: func(base.SubmissionID) (scene.ActionState, error) { return scene.Started, nil },
					Cancel: func(id base.SubmissionID) error {
						cancelled = append(cancelled, id)
						if operation == "cancellation callback" {
							abort()
						}
						return nil
					},
				})
				outer := scene.NewGroup(scene.GroupConfig{
					Items: []scene.Item{{Element: form.Element()}},
					Element: scene.ElementConfig{Enabled: func() bool {
						if operation == "submit" || operation == "replace fields" {
							abort()
						}
						return true
					}},
				})
				root := semanticOwner(t, outer.Element())
				previous, err := form.Submit()
				if err != nil {
					t.Fatal(err)
				}
				if operation == "resolver cancellation" && !root.Handle(input.Key{Rune: 'x'}) {
					t.Fatal("form did not start a deferred sequence")
				}
				interrupt = true
				runInterruptedCallback(t, exit, func() {
					if operation == "submit" {
						_, _ = form.Submit()
					} else {
						form.Set(base.NewConfirm(base.ConfirmConfig{}))
					}
				})
				interrupt = false
				if !slices.Equal(cancelled, []base.SubmissionID{previous.Value}) {
					t.Fatalf("revoked attempt received cancellation %v, want %v", cancelled, previous.Value)
				}
				if form.Settle(previous.Value, base.SubmissionOutcome{}) {
					t.Fatal("revoked work retained settlement authority")
				}
			})
		}
	}
}

func TestSupersededFormCancellationPreservesReentrantSubmission(t *testing.T) {
	for _, operation := range []string{"submit", "replace fields"} {
		t.Run(operation, func(t *testing.T) {
			var form *base.Form
			var previous, replacement base.SubmissionID
			cancelled := 0
			form = base.NewForm(base.FormConfig{
				Submit: func(base.SubmissionID) (scene.ActionState, error) { return scene.Started, nil },
				Cancel: func(id base.SubmissionID) error {
					cancelled++
					if id != previous || form.Submission().ID == id {
						t.Fatal("cancellation observed the attempt before revocation")
					}
					result, err := form.Submit()
					if err != nil {
						t.Error(err)
					}
					replacement = result.Value
					return errors.New("obsolete cancellation failure")
				},
			})
			first, err := form.Submit()
			if err != nil {
				t.Fatal(err)
			}
			previous = first.Value
			if operation == "submit" {
				if _, err := form.Submit(); !errors.Is(err, base.ErrSubmissionStale) {
					t.Fatalf("outer submission error = %v", err)
				}
			} else {
				form.Set(base.NewText(base.TextConfig{}))
			}
			current := form.Submission()
			if cancelled != 1 || current.ID != replacement || current.State != base.SubmissionPending || form.Error() != nil {
				t.Fatalf("reentrant cancellation lost its replacement: %+v, cancelled=%d, error=%v", current, cancelled, form.Error())
			}
		})
	}
}

func TestRejectedFormReplacementKeepsItsPendingAttempt(t *testing.T) {
	field := base.NewText(base.TextConfig{})
	cancelled := 0
	form := base.NewForm(base.FormConfig{
		Fields: []base.Field{field},
		Submit: func(base.SubmissionID) (scene.ActionState, error) { return scene.Started, nil },
		Cancel: func(base.SubmissionID) error { cancelled++; return nil },
	})
	previous, err := form.Submit()
	if err != nil {
		t.Fatal(err)
	}
	if failure := configurationFailure(func() { form.Set(field, field) }); failure == nil {
		t.Fatal("duplicate field relation was accepted")
	}
	if cancelled != 0 || form.Submission().ID != previous.Value || !form.Settle(previous.Value, base.SubmissionOutcome{}) {
		t.Fatal("preflight refusal revoked or cancelled accepted work")
	}
}
