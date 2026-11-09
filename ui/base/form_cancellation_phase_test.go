package base_test

import (
	"errors"
	"testing"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/scene"
)

func TestFormRejectedPureCancellationPreservesFeedback(t *testing.T) {
	problem := errors.New("cancellation refused")
	var form *base.Form
	requests, rejected := 0, 0
	form = base.NewForm(base.FormConfig{
		Submit: func(base.SubmissionID) (scene.ActionState, error) { return scene.Started, nil },
		Cancel: func(base.SubmissionID) error {
			requests++
			form.Element().ReadOnly(func() {
				defer func() {
					if failure, ok := recover().(error); ok && errors.Is(failure, scene.ErrBusy) {
						rejected++
					} else {
						t.Errorf("pure cancellation did not report ErrBusy: %v", failure)
					}
				}()
				form.Do(base.Cancel)
			})
			return problem
		},
	})
	root, err := scene.NewRoot(scene.RootConfig{Element: form.Element()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(root.Close)
	if _, err := form.Submit(); err != nil {
		t.Fatal(err)
	}
	form.Do(base.Cancel)
	if requests != 1 || rejected != 1 {
		t.Fatalf("requests=%d rejected=%d", requests, rejected)
	}
	if !errors.Is(form.Error(), problem) {
		t.Fatalf("rejected read-only request superseded accepted feedback: got %v, want %v", form.Error(), problem)
	}
}
