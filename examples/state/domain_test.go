package main

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/scene"
)

func TestDomainPreservesCancellationCause(t *testing.T) {
	cause := errors.New("request owner ended")
	ctx, cancel := context.WithCancelCause(t.Context())
	cancel(cause)
	domain := newNotificationSettings()
	before := domain.Snapshot()
	result, err := domain.Change(ctx, base.ConfirmRequest{ExpectedRevision: before.Revision, Value: true})
	if !errors.Is(err, cause) || result.State != scene.Unchanged || domain.Snapshot() != before {
		t.Fatalf("cancelled domain change = %+v, %v; snapshot=%+v", result, err, domain.Snapshot())
	}
}
