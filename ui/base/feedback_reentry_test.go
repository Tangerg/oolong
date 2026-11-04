package base_test

import (
	"context"
	"errors"
	"runtime"
	"testing"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/scene"
)

func TestConfirmKeepsFeedbackFromTheNewestReentrantUIRequest(t *testing.T) {
	older := errors.New("earlier request failed")
	newer := errors.New("newer request failed")
	for _, test := range []struct {
		name       string
		old, fresh error
	}{
		{name: "newer success", old: older},
		{name: "newer failure", fresh: newer},
	} {
		for _, source := range []string{"command", "keyboard", "programmatic", "semantic"} {
			t.Run(test.name+"/"+source, func(t *testing.T) {
				domain := &confirmDomain{problem: test.old}
				field := base.NewConfirm(base.ConfirmConfig{Control: domain})
				t.Cleanup(field.Close)
				notify := domain.notify
				nested := false
				domain.notify = func() {
					notify()
					if !nested {
						nested = true
						domain.problem = test.fresh
						field.Say(false)
						domain.problem = test.old
					}
				}
				root := semanticOwner(t, field.Element())
				switch source {
				case "command":
					field.Say(true)
				case "keyboard":
					root.Handle(input.Key{Rune: ' '})
				case "programmatic":
					_, _ = field.Request(t.Context(), base.ConfirmRequest{Value: true})
				case "semantic":
					_, _ = root.Invoke(t.Context(), field.Element().Target(), scene.ActionSetValue, true)
				}
				if !nested || field.Answer() || field.Revision() != 2 {
					t.Fatalf("nested request did not publish the final answer: %+v", field.Snapshot())
				}
				if !errors.Is(field.ChangeError(), test.fresh) {
					t.Fatalf("earlier request replaced newer feedback: got %v, want %v", field.ChangeError(), test.fresh)
				}
			})
		}
	}
}

func TestButtonKeepsFeedbackFromTheNewestReentrantActivation(t *testing.T) {
	older := errors.New("earlier activation failed")
	newer := errors.New("newer activation failed")
	for _, test := range []struct {
		name       string
		old, fresh error
	}{
		{name: "newer success", old: older},
		{name: "newer failure", fresh: newer},
	} {
		t.Run(test.name, func(t *testing.T) {
			var root *scene.Root
			nested := false
			button := base.NewButton(base.ButtonConfig{Activate: func(context.Context) (scene.ActionResult[struct{}], error) {
				if nested {
					return scene.ActionResult[struct{}]{}, test.fresh
				}
				nested = true
				root.Handle(input.Key{Code: input.Enter})
				return scene.ActionResult[struct{}]{}, test.old
			}})
			root = semanticOwner(t, button.Element())
			root.Handle(input.Key{Code: input.Enter})
			if !nested {
				t.Fatal("keyboard activation did not reach the action")
			}
			if !errors.Is(button.Error(), test.fresh) {
				t.Fatalf("earlier activation replaced newer feedback: got %v, want %v", button.Error(), test.fresh)
			}
		})
	}
}

func TestFormKeepsFeedbackFromTheNewestCancellationRequest(t *testing.T) {
	older := errors.New("earlier cancellation failed")
	newer := errors.New("newer cancellation failed")
	for _, test := range []struct {
		name       string
		old, fresh error
	}{
		{name: "newer success", old: older},
		{name: "newer failure", old: older, fresh: newer},
	} {
		t.Run(test.name, func(t *testing.T) {
			var form *base.Form
			nested := false
			form = base.NewForm(base.FormConfig{
				Submit: func(base.SubmissionID) (scene.ActionState, error) { return scene.Started, nil },
				Cancel: func(base.SubmissionID) error {
					if nested {
						return test.fresh
					}
					nested = true
					form.Do(base.Cancel)
					return test.old
				},
			})
			if _, err := form.Submit(); err != nil {
				t.Fatal(err)
			}
			root := semanticOwner(t, form.Element())
			root.Handle(input.Key{Code: input.Esc})
			if !nested || form.Submission().State != base.SubmissionPending {
				t.Fatal("cancellation did not stay with its pending attempt")
			}
			if !errors.Is(form.Error(), test.fresh) {
				t.Fatalf("earlier cancellation replaced newer feedback: got %v, want %v", form.Error(), test.fresh)
			}
		})
	}
}

func TestSuccessfulFormCancellationClearsPreviousUIFeedback(t *testing.T) {
	problem := errors.New("cancellation refused")
	failure := problem
	form := base.NewForm(base.FormConfig{
		Submit: func(base.SubmissionID) (scene.ActionState, error) { return scene.Started, nil },
		Cancel: func(base.SubmissionID) error { return failure },
	})
	if _, err := form.Submit(); err != nil {
		t.Fatal(err)
	}
	form.Do(base.Cancel)
	if !errors.Is(form.Error(), problem) {
		t.Fatal("failed cancellation did not publish its feedback")
	}
	failure = nil
	if err := form.Cancel(); err != nil || !errors.Is(form.Error(), problem) {
		t.Fatal("programmatic cancellation replaced physical feedback")
	}
	form.Do(base.Cancel)
	if form.Error() != nil || form.Submission().State != base.SubmissionPending {
		t.Fatalf("successful cancellation retained obsolete feedback: %v", form.Error())
	}
}

func TestFormCancellationFeedbackEndsWithItsSubmissionAttempt(t *testing.T) {
	for _, transition := range []string{"fields", "submit", "settle"} {
		t.Run(transition, func(t *testing.T) {
			problem := errors.New("cancellation failed")
			form := base.NewForm(base.FormConfig{
				Submit: func(base.SubmissionID) (scene.ActionState, error) { return scene.Started, nil },
				Cancel: func(base.SubmissionID) error { return problem },
			})
			attempt, err := form.Submit()
			if err != nil {
				t.Fatal(err)
			}
			form.Do(base.Cancel)
			if !errors.Is(form.Error(), problem) {
				t.Fatal("failed cancellation did not publish its feedback")
			}
			switch transition {
			case "fields":
				form.Set()
			case "submit":
				if _, err := form.Submit(); err != nil {
					t.Fatal(err)
				}
			case "settle":
				if !form.Settle(attempt.Value, base.SubmissionOutcome{}) {
					t.Fatal("current submission could not settle")
				}
			}
			if form.Error() != nil {
				t.Fatalf("finished attempt retained cancellation feedback: %v", form.Error())
			}
		})
	}
}

func TestInterruptedUIRequestKeepsItsNewerCompletedFeedback(t *testing.T) {
	for _, kind := range []string{"button", "confirm", "form"} {
		for _, exit := range []string{"panic", "Goexit"} {
			t.Run(kind+"/"+exit, func(t *testing.T) {
				problem := errors.New("newer request failed")
				abort := func() {
					if exit == "Goexit" {
						runtime.Goexit()
					}
					panic("interrupted older request")
				}
				var invoke func()
				var feedback func() error
				nested := false
				switch kind {
				case "button":
					var root *scene.Root
					button := base.NewButton(base.ButtonConfig{Activate: func(context.Context) (scene.ActionResult[struct{}], error) {
						if nested {
							return scene.ActionResult[struct{}]{}, problem
						}
						nested = true
						root.Handle(input.Key{Code: input.Enter})
						abort()
						return scene.ActionResult[struct{}]{}, nil
					}})
					root = semanticOwner(t, button.Element())
					invoke, feedback = func() { root.Handle(input.Key{Code: input.Enter}) }, button.Error
				case "confirm":
					domain := &confirmDomain{}
					field := base.NewConfirm(base.ConfirmConfig{Control: domain})
					t.Cleanup(field.Close)
					notify := domain.notify
					domain.notify = func() {
						notify()
						if !nested {
							nested = true
							domain.problem = problem
							field.Say(false)
							abort()
						}
					}
					invoke, feedback = func() { field.Say(true) }, field.ChangeError
				case "form":
					var form *base.Form
					form = base.NewForm(base.FormConfig{
						Submit: func(base.SubmissionID) (scene.ActionState, error) { return scene.Started, nil },
						Cancel: func(base.SubmissionID) error {
							if nested {
								return problem
							}
							nested = true
							form.Do(base.Cancel)
							abort()
							return nil
						},
					})
					if _, err := form.Submit(); err != nil {
						t.Fatal(err)
					}
					invoke, feedback = func() { form.Do(base.Cancel) }, form.Error
				}
				done := make(chan struct{})
				go func() {
					defer close(done)
					defer func() {
						failure := recover()
						if exit == "panic" && failure != "interrupted older request" || exit == "Goexit" && failure != nil {
							t.Errorf("interrupted request exited with %v", failure)
						}
					}()
					invoke()
					t.Error("interrupted request returned normally")
				}()
				<-done
				if !nested || !errors.Is(feedback(), problem) {
					t.Fatalf("abnormal exit changed newer feedback: %v", feedback())
				}
			})
		}
	}
}
