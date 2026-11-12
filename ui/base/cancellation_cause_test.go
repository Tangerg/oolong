package base_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/scene"
)

func TestConfirmationAndSearchPreserveCancellationCause(t *testing.T) {
	cause := errors.New("request owner ended")
	ctx, cancel := context.WithCancelCause(t.Context())
	cancel(cause)
	confirm := base.NewConfirm(base.ConfirmConfig{})
	before := confirm.Snapshot()
	result, err := confirm.Request(ctx, base.ConfirmRequest{ExpectedRevision: before.Revision, Value: true})
	if !errors.Is(err, cause) || result.State != scene.Unchanged || confirm.Snapshot() != before {
		t.Errorf("confirmation cancellation = %+v, %v; snapshot=%+v", result, err, confirm.Snapshot())
	}
	if _, err := base.Scan(ctx, base.ScanConfig{Query: "query"}); !errors.Is(err, cause) {
		t.Errorf("search cancellation = %v", err)
	}
}
