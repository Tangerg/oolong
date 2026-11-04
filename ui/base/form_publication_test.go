package base_test

import (
	"errors"
	"runtime"
	"testing"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/scene"
)

type submissionProbeField struct {
	observedField
	inspect func(string)
}

func (f *submissionProbeField) Validate() error {
	f.inspect("Validate")
	return nil
}

func (f *submissionProbeField) Error() error {
	f.inspect("Error")
	return nil
}

func (f *submissionProbeField) Revision() uint64 {
	f.inspect("Revision")
	return 0
}

func (f *submissionProbeField) Validation() func() bool {
	f.inspect("Validation")
	return func() bool { f.inspect("receipt"); return true }
}

func TestInterruptedFormValidationPublishesItsNewAttempt(t *testing.T) {
	for _, callback := range []string{"Check", "collect", "Validate", "Error", "Validation", "Revision", "receipt"} {
		for _, exit := range []string{"panic", "Goexit"} {
			t.Run(callback+"/"+exit, func(t *testing.T) {
				interrupted := false
				inspect := func(operation string) {
					if !interrupted || operation != callback {
						return
					}
					if exit == "Goexit" {
						runtime.Goexit()
					}
					panic("interrupted validation")
				}
				field := &submissionProbeField{inspect: inspect}
				cancelError := errors.New("cancellation refused")
				starts := 0
				form := base.NewForm(base.FormConfig{
					Fields: []base.Field{field},
					Check:  func() error { inspect("Check"); return nil },
					Submit: func(base.SubmissionID) (scene.ActionState, error) { starts++; return scene.Started, nil },
					Cancel: func(base.SubmissionID) error { return cancelError },
				})
				root := semanticOwner(t, form.Element())
				previous, err := form.Submit()
				if err != nil {
					t.Fatal(err)
				}
				form.Do(base.Cancel)
				if !errors.Is(form.Error(), cancelError) {
					t.Fatal("cancellation feedback was not delivered")
				}
				changed := root.Changes()
				interrupted = true
				runInterruptedFormValidation(t, exit, func() {
					if callback == "collect" || callback == "Error" {
						_, _ = form.SubmitWith(func(field base.Field) error {
							inspect("collect")
							return field.Validate()
						})
					} else {
						_, _ = form.Submit()
					}
				})
				interrupted = false
				attempt := form.Submission()
				if attempt.ID == previous.Value || attempt.ID == (base.SubmissionID{}) || attempt.State != base.SubmissionIdle || attempt.Current || starts != 1 {
					t.Fatalf("interrupted validation did not retain its unaccepted attempt: %+v, starts=%d", attempt, starts)
				}
				if form.Error() != nil || form.Settle(previous.Value, base.SubmissionOutcome{}) || form.Settle(attempt.ID, base.SubmissionOutcome{}) {
					t.Fatal("interrupted validation retained feedback or authorized settlement")
				}
				select {
				case <-changed:
				default:
					t.Fatal("new attempt cleared feedback and pending state without notifying scene")
				}
			})
		}
	}
}

func TestInterruptedFormValidationPreservesItsReentrantReplacement(t *testing.T) {
	for _, replace := range []string{"submit", "fields"} {
		for _, exit := range []string{"panic", "Goexit"} {
			t.Run(replace+"/"+exit, func(t *testing.T) {
				var form *base.Form
				var check func()
				form = base.NewForm(base.FormConfig{
					Check: func() error {
						if check != nil {
							check()
						}
						return nil
					},
					Submit: func(base.SubmissionID) (scene.ActionState, error) { return scene.Started, nil },
				})
				root := semanticOwner(t, form.Element())
				var replacement base.Submission
				check = func() {
					check = nil
					if replace == "submit" {
						if _, err := form.Submit(); err != nil {
							t.Error(err)
						}
					} else {
						form.Set()
					}
					replacement = form.Submission()
					if exit == "Goexit" {
						runtime.Goexit()
					}
					panic("interrupted validation")
				}
				changed := root.Changes()
				runInterruptedFormValidation(t, exit, func() { _, _ = form.Submit() })
				if got := form.Submission(); got != replacement || form.Error() != nil {
					t.Fatalf("interrupted outer validation replaced the newer attempt: got %+v, want %+v", got, replacement)
				}
				select {
				case <-changed:
				default:
					t.Fatal("replacement did not notify scene")
				}
			})
		}
	}
}

func TestFormPublishesValidationFailureAfterItsCallbackReturns(t *testing.T) {
	problem := errors.New("draft rejected")
	var root *scene.Root
	var changed <-chan struct{}
	form := base.NewForm(base.FormConfig{Check: func() error {
		changed = root.Changes()
		return problem
	}})
	root = semanticOwner(t, form.Element())
	if _, err := form.Submit(); !errors.Is(err, problem) || !errors.Is(form.Error(), problem) {
		t.Fatalf("validation problem was not published: %v, %v", err, form.Error())
	}
	select {
	case <-changed:
	default:
		t.Fatal("validation failure did not notify its observer")
	}
}

func runInterruptedFormValidation(t *testing.T, exit string, invoke func()) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer func() {
			failure := recover()
			if exit == "panic" && failure != "interrupted validation" || exit == "Goexit" && failure != nil {
				t.Errorf("validation exited with %v", failure)
			}
		}()
		invoke()
		t.Error("interrupted validation returned normally")
	}()
	<-done
}
