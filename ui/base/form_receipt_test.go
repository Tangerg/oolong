package base_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/internal/scenetest"
)

func TestSpokenConfirmValidatesOnlyAnAcceptedAnswer(t *testing.T) {
	for _, mode := range []string{"closed", "started"} {
		t.Run(mode, func(t *testing.T) {
			checks := 0
			domain := &confirmDomain{asynchronous: true}
			field := base.NewConfirm(base.ConfirmConfig{Control: domain, Check: func(bool) error { checks++; return nil }})
			defer field.Close()
			if mode == "closed" {
				field.Close()
			}
			err := field.Reply("yes")
			if !errors.Is(err, base.ErrReplyUnavailable) || checks != 0 || field.Validation() != nil || field.Error() != nil || field.Answer() {
				t.Fatalf("unaccepted yes: err=%v checks=%d validated=%v answer=%v", err, checks, field.Validation() != nil, field.Answer())
			}
			if mode == "closed" && !errors.Is(err, scene.ErrClosed) {
				t.Fatalf("closed reply = %v", err)
			}
			if mode == "started" {
				if !errors.Is(err, base.ErrConfirmPending) {
					t.Fatalf("pending reply = %v", err)
				}
				domain.publish(true)
				if err := field.Reply("yes"); err != nil || checks != 1 || domain.requests != 1 {
					t.Fatalf("published yes: err=%v checks=%d requests=%d", err, checks, domain.requests)
				}
			}
		})
	}
}

func TestFormRejectsAFailedValidationReplacedByALaterField(t *testing.T) {
	invalid := errors.New("old problem")
	first := base.NewText(base.TextConfig{Check: func(string) error { return invalid }})
	second := base.NewText(base.TextConfig{Check: func(string) error {
		first.SetCheck(nil)
		return first.Validate()
	}})
	form := base.NewForm(base.FormConfig{Fields: []base.Field{first, second}})
	if _, err := form.Submit(); !errors.Is(err, base.ErrValidationSuperseded) || form.Error() != nil || form.Submission().Current {
		t.Fatalf("obsolete validation: err=%v form error=%v attempt=%+v", err, form.Error(), form.Submission())
	}
}

func TestCancelFailureCannotReplaceANewerSubmissionOutcome(t *testing.T) {
	var form *base.Form
	failed := errors.New("old cancellation failed")
	form = base.NewForm(base.FormConfig{
		Submit: func(base.SubmissionID) (scene.ActionState, error) {
			return scene.Started, nil
		},
		Cancel: func(id base.SubmissionID) error {
			if !form.Settle(id, base.SubmissionOutcome{Cancelled: true}) {
				t.Fatal("pending attempt was not settled")
			}
			return failed
		},
	})
	_, _ = form.Submit()
	form.Do(base.Cancel)
	if result := form.Submission(); result.State != base.SubmissionCancelled || result.Err != nil || form.Error() != nil {
		t.Fatalf("settled attempt %+v shows stale problem %v", result, form.Error())
	}
}

func TestFormPaintBelongsToTheFieldRelation(t *testing.T) {
	scenes := scenetest.New()
	t.Cleanup(scenes.Close)

	field := base.NewText(base.TextConfig{Initial: "value"})
	var painted []string
	if err := field.Element().SetPaint(func(frame scene.Frame) {
		painted = append(painted, "standalone")
		field.Draw(frame)
	}); err != nil {
		t.Fatal(err)
	}
	form := base.NewForm(base.FormConfig{Fields: []base.Field{field}, PaintField: func(frame scene.Frame, child base.Field) {
		painted = append(painted, "form")
		child.Draw(frame)
	}})
	_ = paintWidget(scenes, 12, 1, form)
	form.Set()
	scenes.Mount(form).FlushNotifications()
	plain := base.NewForm(base.FormConfig{Fields: []base.Field{field}})
	_ = paintWidget(scenes, 12, 1, plain)
	if !slices.Equal(painted, []string{"form", "standalone"}) {
		t.Fatalf("field painters before and after transfer = %v", painted)
	}
}
