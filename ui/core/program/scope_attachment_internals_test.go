package program

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"

	"github.com/Tangerg/oolong/ui/core/scene"
)

func TestScopeRejectsSuppliedInvalidAttachments(t *testing.T) {
	owner, runtime := taskOwner(t)
	element := owner.root.Content()
	mounted := element.Attachment()
	if err := owner.root.SetContent(scene.NewElement(scene.ElementConfig{})); err != nil {
		t.Fatal(err)
	}
	foreign, _ := taskOwner(t)
	for _, test := range []struct {
		name  string
		lease scene.Attachment
	}{
		{name: "empty"},
		{name: "before mounting", lease: scene.NewElement(scene.ElementConfig{}).Attachment()},
		{name: "ended mount", lease: mounted},
		{name: "after detachment", lease: element.Attachment()},
		{name: "another runtime", lease: foreign.root.Content().Attachment()},
	} {
		t.Run(test.name, func(t *testing.T) {
			scope, err := runtime.NewScope(ScopeConfig{Attachment: &test.lease})
			if scope != nil {
				scope.Close()
				t.Fatal("invalid attachment acquired a scope")
			}
			if !errors.Is(err, ErrScopeClosed) {
				t.Fatalf("NewScope error = %v, want ErrScopeClosed", err)
			}
		})
	}
}

func TestScopeWithoutAttachmentFollowsTheRuntime(t *testing.T) {
	owner, runtime := taskOwner(t)
	scope, err := runtime.NewScope(ScopeConfig{})
	if err != nil {
		t.Fatal(err)
	}
	defer scope.Close()
	if err = owner.root.SetContent(scene.NewElement(scene.ElementConfig{})); err != nil {
		t.Fatal(err)
	}
	var applied bool
	task, err := runtime.Start(scope, Work[int]{
		Run:   func(context.Context) (int, error) { return 42, nil },
		Apply: func(int, error) { applied = true },
	})
	if err != nil {
		t.Fatal(err)
	}
	settleTask(t, owner, task)
	if !applied || task.Snapshot().Delivery != DeliveryApplied {
		t.Fatal("changing the scene revoked a session-bound scope")
	}
}

func TestScopeRetainsItsMountLeaseAfterTheCallerChangesItsCopy(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		owner, runtime := taskOwner(t)
		lease := owner.root.Content().Attachment()
		scope, err := runtime.NewScope(ScopeConfig{Attachment: &lease})
		if err != nil {
			t.Fatal(err)
		}
		defer scope.Close()
		lease = scene.Attachment{}
		var applied bool
		task, err := runtime.Start(scope, Work[int]{
			Run:   func(context.Context) (int, error) { return 42, nil },
			Apply: func(int, error) { applied = true },
		})
		if err != nil {
			t.Fatal(err)
		}
		<-task.Done()
		<-owner.tasks.wake
		if err := owner.root.SetContent(scene.NewElement(scene.ElementConfig{})); err != nil {
			t.Fatal(err)
		}
		owner.runTasks()
		synctest.Wait()
		if status := task.Snapshot(); applied || status.Delivery != DeliveryRevoked || status.State != TaskSucceeded {
			t.Fatalf("detached completion = %+v, applied = %v", status, applied)
		}
		if !errors.Is(context.Cause(scope.Context()), ErrScopeClosed) {
			t.Fatalf("scope cancellation = %v, want ErrScopeClosed", context.Cause(scope.Context()))
		}
		if _, err := runtime.Start(scope, Work[int]{Run: func(context.Context) (int, error) { return 1, nil }}); !errors.Is(err, ErrScopeClosed) {
			t.Fatalf("start after detachment = %v, want ErrScopeClosed", err)
		}
	})
}
