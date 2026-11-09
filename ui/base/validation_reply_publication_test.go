package base_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/scene"
)

type rejectingReplyControl struct {
	beforeReject func()
	rejection    error
}

func (*rejectingReplyControl) Snapshot() base.ConfirmSnapshot { return base.ConfirmSnapshot{} }
func (*rejectingReplyControl) Subscribe(func()) func()        { return func() {} }
func (c *rejectingReplyControl) Change(context.Context, base.ConfirmRequest) (scene.ActionResult[base.ConfirmSnapshot], error) {
	if c.beforeReject != nil {
		c.beforeReject()
	}
	return scene.ActionResult[base.ConfirmSnapshot]{Value: c.Snapshot()}, c.rejection
}

func TestConfirmReplyRejectionPublishesValidationRevocation(t *testing.T) {
	for _, reenter := range []bool{false, true} {
		name := "plain rejection"
		if reenter {
			name = "rejection after newer no-op UI request"
		}
		t.Run(name, func(t *testing.T) {
			oldProblem := errors.New("previous answer is invalid")
			rejection := errors.New("requested answer refused")
			domain := &rejectingReplyControl{rejection: rejection}
			field := base.NewConfirm(base.ConfirmConfig{Control: domain, Check: func(bool) error { return oldProblem }})
			t.Cleanup(field.Close)
			root, err := scene.NewRoot(scene.RootConfig{Element: field.Element()})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(root.Close)
			if err = field.Validate(); !errors.Is(err, oldProblem) {
				t.Fatal(err)
			}
			receipt := field.Validation()
			if receipt == nil || !receipt() {
				t.Fatal("fixture has no completed validation receipt")
			}
			if reenter {
				domain.beforeReject = func() { field.Say(false) }
			}
			before, changed := root.Progress().ModelRevision, root.Changes()
			err = field.Reply("yes")
			if !errors.Is(err, base.ErrReplyUnavailable) || !errors.Is(err, rejection) {
				t.Fatalf("reply error=%v", err)
			}
			if field.Answer() || field.Error() != nil || receipt() || field.Validation() != nil {
				t.Fatalf("unexpected answer/validation state: answer=%v error=%v receipt=%v", field.Answer(), field.Error(), receipt())
			}
			select {
			case <-changed:
				t.Logf("rejected reply published validation revocation: revision %d -> %d", before, root.Progress().ModelRevision)
			default:
				t.Fatalf("normal rejected reply revoked validation without notification: revision %d -> %d; feedback=%v", before, root.Progress().ModelRevision, field.ChangeError())
			}
		})
	}
}
