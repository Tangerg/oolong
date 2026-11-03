package base_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/scene"
)

func TestButtonPressPublishesEffectsIndependentlyOfErrors(t *testing.T) {
	problem := errors.New("notification failed after activation")
	for _, test := range []struct {
		name  string
		state scene.ActionState
		err   error
	}{
		{"unchanged", scene.Unchanged, nil},
		{"rejected", scene.Unchanged, problem},
		{"applied", scene.Applied, nil},
		{"partially applied", scene.Applied, problem},
		{"started", scene.Started, nil},
		{"partially started", scene.Started, problem},
	} {
		t.Run(test.name, func(t *testing.T) {
			button := base.NewButton(base.ButtonConfig{Activate: func(context.Context) (scene.ActionResult[struct{}], error) {
				return scene.ActionResult[struct{}]{State: test.state}, test.err
			}})
			root := semanticOwner(t, button.Element())
			before, changed := root.Progress().ModelRevision, root.Changes()
			result, err := button.Press(t.Context())
			if result.State != test.state || !errors.Is(err, test.err) || button.Error() != nil {
				t.Fatalf("activation = %+v, %v, UI error = %v", result, err, button.Error())
			}
			assertModelPublication(t, root, before, changed, test.state != scene.Unchanged)
		})
	}
}

func TestButtonInputPublishesFeedbackBeforeScenePredicates(t *testing.T) {
	problem := errors.New("activation failed")
	for _, source := range []string{"command", "keyboard", "pointer"} {
		t.Run(source, func(t *testing.T) {
			var outcome scene.ActionState
			var failure error
			invoked, observed := false, false
			button := base.NewButton(base.ButtonConfig{Label: "Run", Activate: func(context.Context) (scene.ActionResult[struct{}], error) {
				invoked = true
				return scene.ActionResult[struct{}]{State: outcome}, failure
			}})
			if err := button.Element().SetFocusable(func() bool {
				if invoked {
					observed = true
					if !errors.Is(button.Error(), failure) {
						t.Errorf("predicate saw error %v before feedback %v was installed", button.Error(), failure)
					}
				}
				return true
			}); err != nil {
				t.Fatal(err)
			}
			root := semanticOwner(t, button.Element())
			root.FlushNotifications()
			frame, err := root.Prepare(grid.NewSurface(10, 1).View())
			if err != nil {
				t.Fatal(err)
			}
			frame.Commit(1)
			root.Wrote(1)
			for _, step := range []struct {
				name    string
				state   scene.ActionState
				err     error
				publish bool
			}{
				{"failure", scene.Unchanged, problem, true},
				{"clear on no-op", scene.Unchanged, nil, true},
				{"partial effect", scene.Applied, problem, true},
				{"clear on effect", scene.Applied, nil, true},
				{"no-op", scene.Unchanged, nil, false},
			} {
				t.Run(step.name, func(t *testing.T) {
					outcome, failure = step.state, step.err
					invoked, observed = false, false
					before, changed := root.Progress().ModelRevision, root.Changes()
					switch source {
					case "command":
						button.Do(base.Activate)
					case "keyboard":
						root.Handle(input.Key{Code: input.Enter})
					case "pointer":
						root.Handle(input.Mouse{Action: input.MouseDown, Button: input.ButtonLeft})
						root.Handle(input.Mouse{Action: input.MouseUp, Button: input.ButtonLeft})
					}
					if !invoked || !errors.Is(button.Error(), failure) || step.publish && !observed {
						t.Fatalf("invoked=%t observed=%t error=%v want=%v", invoked, observed, button.Error(), failure)
					}
					assertModelPublication(t, root, before, changed, step.publish)
				})
			}
			invoked = false
		})
	}
}

func TestButtonProgrammaticActivationPreservesPhysicalFeedback(t *testing.T) {
	problem := errors.New("physical activation failed")
	var outcome scene.ActionState
	failure := problem
	button := base.NewButton(base.ButtonConfig{Activate: func(context.Context) (scene.ActionResult[struct{}], error) {
		return scene.ActionResult[struct{}]{State: outcome}, failure
	}})
	button.Do(base.Activate)
	root := semanticOwner(t, button.Element())
	outcome, failure = scene.Applied, nil
	if _, err := button.Press(t.Context()); err != nil || !errors.Is(button.Error(), problem) {
		t.Fatalf("direct activation changed physical feedback: %v, %v", err, button.Error())
	}
	if _, err := root.Invoke(t.Context(), button.Element().Target(), scene.ActionActivate, nil); err != nil || !errors.Is(button.Error(), problem) {
		t.Fatalf("agent activation changed physical feedback: %v, %v", err, button.Error())
	}
}

func TestConfirmPublishesRejectedAndClearedFeedbackWithoutAValueChange(t *testing.T) {
	problem := errors.New("domain declined the change")
	domain := &confirmDomain{reject: true, problem: problem}
	confirm := base.NewConfirm(base.ConfirmConfig{Control: domain})
	t.Cleanup(confirm.Close)
	root := semanticOwner(t, confirm.Element())
	for _, step := range []struct {
		value   bool
		err     error
		publish bool
	}{{true, problem, true}, {false, nil, true}, {false, nil, false}} {
		before, changed := root.Progress().ModelRevision, root.Changes()
		confirm.Say(step.value)
		if confirm.Answer() || !errors.Is(confirm.ChangeError(), step.err) {
			t.Fatalf("answer=%t error=%v want=%v", confirm.Answer(), confirm.ChangeError(), step.err)
		}
		assertModelPublication(t, root, before, changed, step.publish)
	}
}

func TestConfirmPublishesSynchronousAnswerAndFeedbackTogether(t *testing.T) {
	previous := errors.New("previous rejection")
	problem := errors.New("notification failed after the domain changed")
	for _, failure := range []error{nil, problem} {
		domain := &confirmDomain{reject: true, problem: previous}
		confirm := base.NewConfirm(base.ConfirmConfig{Control: domain})
		t.Cleanup(confirm.Close)
		confirm.Say(true)
		checking, observed := false, false
		if err := confirm.Element().SetFocusable(func() bool {
			if checking {
				observed = true
				if !confirm.Answer() || !errors.Is(confirm.ChangeError(), failure) {
					t.Errorf("predicate saw an unsettled answer/feedback: %t, %v", confirm.Answer(), confirm.ChangeError())
				}
			}
			return true
		}); err != nil {
			t.Fatal(err)
		}
		root := semanticOwner(t, confirm.Element())
		domain.reject, domain.problem = false, failure
		before, changed := root.Progress().ModelRevision, root.Changes()
		checking = true
		confirm.Say(true)
		checking = false
		if !observed || !confirm.Answer() || !errors.Is(confirm.ChangeError(), failure) {
			t.Fatalf("observed=%t answer=%t error=%v", observed, confirm.Answer(), confirm.ChangeError())
		}
		assertModelPublication(t, root, before, changed, true)
	}
}

func TestConfirmRequestPublishesAcceptedOutcomesAndReturnsErrorsDirectly(t *testing.T) {
	previous := errors.New("physical request failed")
	problem := errors.New("accepted request has a later failure")
	for _, asynchronous := range []bool{false, true} {
		domain := &confirmDomain{reject: true, problem: previous}
		confirm := base.NewConfirm(base.ConfirmConfig{Control: domain})
		t.Cleanup(confirm.Close)
		confirm.Say(true)
		root := semanticOwner(t, confirm.Element())
		domain.reject, domain.problem, domain.asynchronous = false, problem, asynchronous
		before, changed := root.Progress().ModelRevision, root.Changes()
		result, err := confirm.Request(t.Context(), base.ConfirmRequest{Value: true})
		want := scene.Applied
		if asynchronous {
			want = scene.Started
		}
		wantAnswer := !asynchronous
		if result.State != want || !errors.Is(err, problem) || !errors.Is(confirm.ChangeError(), previous) || confirm.Answer() != wantAnswer {
			t.Fatalf("request = %+v, %v, UI error=%v answer=%t", result, err, confirm.ChangeError(), confirm.Answer())
		}
		assertModelPublication(t, root, before, changed, true)
	}
}

func TestConfirmCoalescesSynchronousNestedDomainNotifications(t *testing.T) {
	domain := &confirmDomain{}
	confirm := base.NewConfirm(base.ConfirmConfig{Control: domain})
	t.Cleanup(confirm.Close)
	notify, nested := domain.notify, false
	domain.notify = func() {
		notify()
		if !nested {
			nested = true
			confirm.Say(false)
		}
	}
	checking, observed := false, false
	if err := confirm.Element().SetFocusable(func() bool {
		if checking {
			observed = true
			if confirm.Answer() || confirm.Revision() != 2 {
				t.Errorf("predicate saw an intermediate domain snapshot: %+v", confirm.Snapshot())
			}
		}
		return true
	}); err != nil {
		t.Fatal(err)
	}
	root := semanticOwner(t, confirm.Element())
	before, changed := root.Progress().ModelRevision, root.Changes()
	checking = true
	result, err := confirm.Request(t.Context(), base.ConfirmRequest{Value: true})
	checking = false
	if err != nil || result.State != scene.Applied || !observed || confirm.Answer() || confirm.Revision() != 2 {
		t.Fatalf("nested request = %+v, %v, observed=%t snapshot=%+v", result, err, observed, confirm.Snapshot())
	}
	assertModelPublication(t, root, before, changed, true)
}

func assertModelPublication(t *testing.T, root *scene.Root, before uint64, changed <-chan struct{}, publish bool) {
	t.Helper()
	want := before
	if publish {
		want++
	}
	if got := root.Progress().ModelRevision; got != want {
		t.Fatalf("model revision = %d, want %d", got, want)
	}
	select {
	case <-changed:
		if !publish {
			t.Fatal("no-op published a scene change")
		}
	default:
		if publish {
			t.Fatal("completed transition did not notify scene observers")
		}
	}
}
