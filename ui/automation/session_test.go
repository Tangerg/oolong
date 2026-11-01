package automation_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/oolong/ui/automation"
	"github.com/Tangerg/oolong/ui/core/program"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/uitest"
)

func TestDriverFollowsItsRuntimeThroughShutdown(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	ready := make(chan *automation.Driver, 1)
	failure := errors.New("follow-up operation failed")
	screen, err := program.NewScreen(program.ScreenConfig{Host: uitest.New(t, uitest.Config{Width: 3, Height: 1}), Build: func(rt *program.Runtime) (scene.Element, error) {
		ready <- automation.NewDriver(rt)
		unchanged := scene.NewAction(scene.ActionConfig[struct{}, struct{}]{ID: scene.ActionActivate, Run: func(context.Context, struct{}) (scene.ActionResult[struct{}], error) {
			return scene.ActionResult[struct{}]{State: scene.Unchanged}, nil
		}})
		partial := scene.NewAction(scene.ActionConfig[scene.ActionState, struct{}]{ID: scene.ActionSetValue, Run: func(_ context.Context, state scene.ActionState) (scene.ActionResult[struct{}], error) {
			return scene.ActionResult[struct{}]{State: state}, failure
		}})
		return scene.NewElement(scene.ElementConfig{Key: "session", Actions: []scene.ActionBinding{unchanged.Binding(), partial.Binding()}}), nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	stopped := make(chan error, 1)
	go func() { stopped <- screen.Run(ctx) }()
	var d *automation.Driver
	select {
	case d = <-ready:
	case err = <-stopped:
		t.Fatal("session stopped during construction", err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	nodes, err := d.Observe(ctx, scene.Query{})
	if err != nil || len(nodes.Nodes) != 1 || nodes.Nodes[0].Key != "session" {
		t.Fatal(nodes, err)
	}
	if _, err = d.Locate(scene.Query{}, scene.Match{Key: "missing"}).Invoke(ctx, scene.ActionActivate, nil); !errors.Is(err, scene.ErrNotFound) {
		t.Fatal(err)
	}
	if result, invokeErr := d.Invoke(ctx, nodes.Nodes[0].Target, scene.ActionActivate, nil); invokeErr != nil || result.State != scene.Unchanged {
		t.Fatal(result, invokeErr)
	}
	after, err := d.Observe(ctx, scene.Query{})
	if err != nil || after.Progress.ModelRevision != nodes.Progress.ModelRevision {
		t.Fatalf("read-only outcomes advanced revision %d to %d: %v", nodes.Progress.ModelRevision, after.Progress.ModelRevision, err)
	}
	checkPartialActions(ctx, t, d, after, failure)
	cancel()
	<-stopped
	if _, err = d.Observe(t.Context(), scene.Query{}); !errors.Is(err, program.ErrStopped) {
		t.Fatalf("stale session observation: %v", err)
	}
}

func checkPartialActions(ctx context.Context, t *testing.T, d *automation.Driver, current scene.Observation, failure error) {
	t.Helper()
	for _, tc := range []struct {
		report, want scene.ActionState
		advance      uint64
	}{
		{scene.Unchanged, scene.Unchanged, 0},
		{scene.Applied, scene.Applied, 1},
		{scene.Started, scene.Started, 1},
		{scene.ActionState(255), scene.Unchanged, 0},
	} {
		result, err := d.Invoke(ctx, current.Nodes[0].Target, scene.ActionSetValue, tc.report)
		if result.State != tc.want || !errors.Is(err, failure) || errors.Is(err, scene.ErrInvalid) != (tc.report != tc.want) {
			t.Fatalf("reported %d: got %+v, %v", tc.report, result, err)
		}
		after, err := d.Observe(ctx, scene.Query{})
		if err != nil || after.Progress.ModelRevision != current.Progress.ModelRevision+tc.advance {
			t.Fatalf("reported %d: revision %d -> %d: %v", tc.report, current.Progress.ModelRevision, after.Progress.ModelRevision, err)
		}
		current = after
	}
}
