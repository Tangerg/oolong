package automation_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/Tangerg/oolong/ui/automation"
	"github.com/Tangerg/oolong/ui/core/scene"
)

type owner struct {
	mu       sync.Mutex
	done     chan struct{}
	observed chan struct{}
}

func (o *owner) Inspect(ctx context.Context, fn func() error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	err := fn()
	select {
	case o.observed <- struct{}{}:
	default:
	}
	return err
}

func (o *owner) Invoke(ctx context.Context, fn func() error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	return fn()
}
func (o *owner) Done() <-chan struct{} { return o.done }

func TestWaitObservesRealModelChangesWithoutPolling(t *testing.T) {
	value := "pending"
	finish := scene.NewAction(scene.ActionConfig[struct{}, struct{}]{ID: scene.ActionActivate, Run: func(context.Context, struct{}) (scene.ActionResult[struct{}], error) {
		value = "finished"
		return scene.ActionResult[struct{}]{State: scene.Applied}, nil
	}})
	e := scene.NewElement(scene.ElementConfig{Key: "job", Actions: []scene.ActionBinding{finish.Binding()}, Semantics: func() scene.Semantic { return scene.Semantic{Role: scene.RoleStatus, Name: "Job", Value: value} }})
	r, err := scene.NewRoot(scene.RootConfig{Element: e})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	o := &owner{done: make(chan struct{}), observed: make(chan struct{}, 1)}
	driver := automation.NewDriver(automation.DriverConfig{Root: r, Owner: o})
	result := make(chan error, 1)
	go func() {
		_, err := driver.Wait(context.Background(), scene.Query{}, func(observation scene.Observation) bool {
			return len(observation.Nodes) == 1 && observation.Nodes[0].Value == "finished"
		})
		result <- err
	}()
	<-o.observed
	if _, err := driver.Invoke(context.Background(), e.Target(), scene.ActionActivate, nil); err != nil {
		t.Fatal(err)
	}
	if err := <-result; err != nil {
		t.Fatal(err)
	}
}

func TestTargetExpiresWhileLocatorResolvesReplacement(t *testing.T) {
	called := 0
	activate := scene.NewAction(scene.ActionConfig[struct{}, int]{ID: scene.ActionActivate, Run: func(context.Context, struct{}) (scene.ActionResult[int], error) {
		called++
		return scene.ActionResult[int]{State: scene.Applied, Value: called}, nil
	}})
	button := func() scene.Element {
		return scene.NewElement(scene.ElementConfig{Key: "save", Actions: []scene.ActionBinding{activate.Binding()}, Semantics: func() scene.Semantic { return scene.Semantic{Role: scene.RoleButton, Name: "Save"} }})
	}
	old := button()
	group := scene.NewGroup(scene.GroupConfig{Items: []scene.Item{{Key: "save", Element: old}}})
	r, err := scene.NewRoot(scene.RootConfig{Element: group.Element()})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	o := &owner{done: make(chan struct{}), observed: make(chan struct{}, 1)}
	driver := automation.NewDriver(automation.DriverConfig{Root: r, Owner: o})
	target, err := driver.Find(context.Background(), scene.Query{}, scene.Match{Key: "save"})
	if err != nil {
		t.Fatal(err)
	}
	locator := driver.Locate(scene.Query{}, scene.Match{Key: "save"})
	if err := o.Invoke(context.Background(), func() error { return group.Set(scene.Item{Key: "save", Element: button()}) }); err != nil {
		t.Fatal(err)
	}
	if _, err := driver.Invoke(context.Background(), target, scene.ActionActivate, nil); !errors.Is(err, scene.ErrStale) {
		t.Fatalf("stale handle: %v", err)
	}
	if _, err := locator.Invoke(context.Background(), scene.ActionActivate, nil); err != nil {
		t.Fatal(err)
	}
	if called != 1 {
		t.Fatalf("action executed %d times", called)
	}
}

func TestLocatorRejectsAmbiguityAndTruncatedEvidence(t *testing.T) {
	button := func() scene.Element {
		return scene.NewElement(scene.ElementConfig{Semantics: func() scene.Semantic { return scene.Semantic{Role: scene.RoleButton, Name: "Delete"} }})
	}
	group := scene.NewGroup(scene.GroupConfig{Items: []scene.Item{{Element: button()}, {Element: button()}}})
	r, err := scene.NewRoot(scene.RootConfig{Element: group.Element()})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	o := &owner{done: make(chan struct{}), observed: make(chan struct{}, 1)}
	driver := automation.NewDriver(automation.DriverConfig{Root: r, Owner: o})
	if _, err := driver.Find(context.Background(), scene.Query{}, scene.Match{Role: scene.RoleButton, Name: "Delete"}); !errors.Is(err, scene.ErrAmbiguous) {
		t.Fatalf("ambiguous: %v", err)
	}
	if _, err := driver.Find(context.Background(), scene.Query{Limit: 1}, scene.Match{Role: scene.RoleButton, Name: "Delete"}); err == nil {
		t.Fatal("truncation was mistaken for uniqueness")
	}
}

func TestWaitHonorsCancellationWithoutChangingModel(t *testing.T) {
	r, err := scene.NewRoot(scene.RootConfig{})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	o := &owner{done: make(chan struct{}), observed: make(chan struct{}, 1)}
	driver := automation.NewDriver(automation.DriverConfig{Root: r, Owner: o})
	before := r.Progress()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := driver.Wait(ctx, scene.Query{}, func(scene.Observation) bool { return false })
		done <- err
	}()
	<-o.observed
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("wait cancellation: %v", err)
	}
	if r.Progress() != before {
		t.Fatal("pure observation advanced the model")
	}
}
