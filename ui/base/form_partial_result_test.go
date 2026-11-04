package base_test

import (
	"errors"
	"testing"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/scene"
)

func TestSubmissionPreservesEffectsAlongsideFailure(t *testing.T) {
	for _, test := range []struct {
		name  string
		state scene.ActionState
	}{
		{name: "unchanged", state: scene.Unchanged},
		{name: "applied", state: scene.Applied},
		{name: "started", state: scene.Started},
	} {
		t.Run(test.name, func(t *testing.T) {
			problem := errors.New("follow-up failed")
			var accepted base.SubmissionID
			form := base.NewForm(base.FormConfig{Submit: func(id base.SubmissionID) (scene.ActionState, error) {
				accepted = id
				return test.state, problem
			}})
			result, err := form.Submit()
			if result.State != test.state || result.Value != accepted || !errors.Is(err, problem) {
				t.Fatalf("result=%+v error=%v, want state=%v id=%v and callback failure", result, err, test.state, accepted)
			}
			if test.state == scene.Started {
				if got := form.Submission(); got.State != base.SubmissionPending || !errors.Is(got.Err, problem) {
					t.Fatalf("accepted work lost its pending receipt: %+v", got)
				}
				if !form.Settle(result.Value, base.SubmissionOutcome{}) || form.Submission().State != base.SubmissionSucceeded {
					t.Fatal("accepted work could not settle after its accompanying error")
				}
			} else if got := form.Submission(); got.State != base.SubmissionFailed || !errors.Is(got.Err, problem) {
				t.Fatalf("synchronous failure receipt=%+v", got)
			}
		})
	}
}

func TestSupersededSubmissionStillReportsItsAcceptedEffects(t *testing.T) {
	problem := errors.New("accepted before replacement")
	var form *base.Form
	var accepted base.SubmissionID
	form = base.NewForm(base.FormConfig{Submit: func(id base.SubmissionID) (scene.ActionState, error) {
		accepted = id
		form.Set()
		return scene.Started, problem
	}})
	result, err := form.Submit()
	if result.State != scene.Started || result.Value != accepted || !errors.Is(err, problem) || !errors.Is(err, base.ErrSubmissionStale) {
		t.Fatalf("superseded accepted work lost its effects: %+v, %v", result, err)
	}
	if form.Settle(accepted, base.SubmissionOutcome{}) || form.Submission().State != base.SubmissionIdle {
		t.Fatal("superseded receipt changed the replacement form")
	}
}
