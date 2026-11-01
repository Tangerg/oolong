package base_test

import (
	"errors"
	"runtime"
	"testing"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/scene"
)

func TestSubmissionReceiptsCannotSettleADifferentForm(t *testing.T) {
	start := func(base.SubmissionID) (scene.ActionState, error) { return scene.Started, nil }
	a, b := base.NewForm(base.FormConfig{Submit: start}), base.NewForm(base.FormConfig{Submit: start})
	x, err := a.Submit()
	if err != nil {
		t.Fatal(err)
	}
	y, err := b.Submit()
	if err != nil {
		t.Fatal(err)
	}
	if x.Value == y.Value || b.Settle(x.Value, base.SubmissionOutcome{}) || a.Settle(y.Value, base.SubmissionOutcome{}) {
		t.Fatal("foreign receipt authorized settlement")
	}
	if !a.Settle(x.Value, base.SubmissionOutcome{}) || !b.Settle(y.Value, base.SubmissionOutcome{}) {
		t.Fatal("current receipts refused")
	}
}

func TestOnlyAcceptedWorkCanBeSettledAsynchronously(t *testing.T) {
	var form *base.Form
	form = base.NewForm(base.FormConfig{Submit: func(id base.SubmissionID) (scene.ActionState, error) {
		if form.Settle(id, base.SubmissionOutcome{Err: errors.New("foreign completion")}) {
			t.Error("a callback still deciding whether to start was already settleable")
		}
		return scene.Started, nil
	}})
	result, err := form.Submit()
	if err != nil || result.State != scene.Started || form.Submission().State != base.SubmissionPending {
		t.Fatal(result, err, form.Submission())
	}
}

func TestInterruptedSubmitDoesNotLeaveImaginaryPendingWork(t *testing.T) {
	for _, phase := range []string{"panic", "exit"} {
		t.Run(phase, func(t *testing.T) {
			var id base.SubmissionID
			form := base.NewForm(base.FormConfig{Submit: func(v base.SubmissionID) (scene.ActionState, error) {
				id = v
				if phase == "exit" {
					runtime.Goexit()
				}
				panic("start")
			}})
			done := make(chan struct{})
			go func() {
				defer close(done)
				defer func() {
					p := recover()
					if phase == "panic" && p != "start" || phase == "exit" && p != nil {
						t.Errorf("panic %v", p)
					}
				}()
				_, _ = form.Submit()
			}()
			<-done
			if got := form.Submission(); got.State != base.SubmissionFailed || got.Err == nil || got.ID != id || form.Settle(id, base.SubmissionOutcome{}) {
				t.Fatalf("interrupted dispatch %+v", got)
			}
		})
	}
}

func TestFieldEvidenceDoesNotMarkTheReplacementDraftInvalid(t *testing.T) {
	missing := errors.New("missing")
	field := base.NewText(base.TextConfig{Check: func(v string) error {
		if v == "" {
			return missing
		}
		return nil
	}})
	form := base.NewForm(base.FormConfig{Fields: []base.Field{field}})
	if _, err := form.Submit(); !errors.Is(err, missing) {
		t.Fatal(err)
	}
	field.SetText("available")
	if field.Error() != nil || form.Error() != nil {
		t.Errorf("field/form still display the rejected old answer: %v, %v", field.Error(), form.Error())
	}
}

func TestFormShowsCurrentFieldProblemsAfterAnUnrelatedDraftChanges(t *testing.T) {
	bad := func(problem error) *base.Text {
		return base.NewText(base.TextConfig{Check: func(s string) error {
			if s == "" {
				return problem
			}
			return nil
		}})
	}
	first, second := errors.New("first"), errors.New("second")
	a, b, c := bad(first), bad(second), base.NewText(base.TextConfig{})
	form := base.NewForm(base.FormConfig{Fields: []base.Field{a, b, c}})
	if _, err := form.Submit(); !errors.Is(err, first) {
		t.Fatal(err)
	}
	c.SetText("unrelated")
	if err := form.Error(); !errors.Is(err, first) {
		t.Fatalf("unrelated edit cleared a current error: %v", err)
	}
	a.SetText("valid")
	if err := form.Error(); !errors.Is(err, second) {
		t.Fatalf("next current field problem: %v", err)
	}
}
