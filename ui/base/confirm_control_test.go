package base_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/scene"
)

type confirmDomain struct {
	snapshot     base.ConfirmSnapshot
	notify       func()
	pending      *base.ConfirmRequest
	asynchronous bool
	requests     int
	closed       int
}

func (d *confirmDomain) Snapshot() base.ConfirmSnapshot { return d.snapshot }
func (d *confirmDomain) Change(_ context.Context, request base.ConfirmRequest) (scene.ActionResult[base.ConfirmSnapshot], error) {
	d.requests++
	if request.ExpectedRevision != d.snapshot.Revision {
		return scene.ActionResult[base.ConfirmSnapshot]{}, base.ErrConfirmStale
	}
	if d.asynchronous {
		d.pending = &request
		return scene.ActionResult[base.ConfirmSnapshot]{State: scene.Started, Value: d.snapshot}, nil
	}
	d.publish(request.Value)
	return scene.ActionResult[base.ConfirmSnapshot]{State: scene.Applied, Value: d.snapshot}, nil
}

func (d *confirmDomain) Subscribe(notify func()) func() {
	d.notify = notify
	return func() { d.closed++; d.notify = nil }
}

func (d *confirmDomain) publish(value bool) {
	d.snapshot = base.ConfirmSnapshot{Value: value, Revision: d.snapshot.Revision + 1}
	if d.notify != nil {
		d.notify()
	}
}

func TestConfirmControlPublishesVersionsWithoutALocalMirror(t *testing.T) {
	domain := &confirmDomain{snapshot: base.ConfirmSnapshot{Revision: 10}}
	field := base.NewConfirm(base.ConfirmConfig{Label: "Live", Control: domain})
	defer field.Close()
	root := newTestRoot(field)
	root.Draw(grid.NewSurface(12, 2).View())
	wake := root.root.Changes()
	domain.publish(true)
	select {
	case <-wake:
	default:
		t.Fatal("domain notification did not invalidate scene")
	}
	if !field.Answer() || field.Revision() != 11 {
		t.Fatal("field did not read the domain's atomic snapshot")
	}
	observation, err := root.root.Observe(scene.Query{})
	if err != nil || len(observation.Nodes) != 1 || observation.Nodes[0].Value != "true" {
		t.Fatalf("semantic snapshot=%v err=%v", observation, err)
	}
	result, err := field.Request(context.Background(), base.ConfirmRequest{ExpectedRevision: 10, Value: false})
	if !errors.Is(err, base.ErrConfirmStale) || result.State != scene.Unchanged || domain.requests != 0 || !field.Answer() {
		t.Fatal("stale request reached domain or overwrote value")
	}
	root.Handle(input.Key{Rune: ' '})
	if field.Answer() || domain.snapshot.Value || domain.requests != 1 {
		t.Fatal("keyboard wrote a local mirror instead of the domain")
	}
	invoked, err := root.root.Invoke(context.Background(), field.Element().Target(), scene.ActionSetValue, true)
	if err != nil || invoked.State != scene.Applied || !field.Answer() || domain.requests != 2 {
		t.Fatalf("AX request=%v err=%v", invoked, err)
	}
}

func TestConfirmControlStartedWaitsForDomainPublication(t *testing.T) {
	domain := &confirmDomain{snapshot: base.ConfirmSnapshot{Revision: 1}, asynchronous: true}
	field := base.NewConfirm(base.ConfirmConfig{Control: domain})
	result, err := field.Request(context.Background(), base.ConfirmRequest{ExpectedRevision: 1, Value: true})
	if err != nil || result.State != scene.Started || field.Answer() || domain.pending == nil {
		t.Fatalf("pending result=%v err=%v", result, err)
	}
	domain.publish(domain.pending.Value)
	if !field.Answer() || field.Revision() != 2 {
		t.Fatal("completed domain version was not visible")
	}
	field.Close()
	field.Close()
	if domain.closed != 1 {
		t.Fatal("subscription cancellation was not idempotent")
	}
	_, err = field.Request(context.Background(), base.ConfirmRequest{ExpectedRevision: 2, Value: false})
	if !errors.Is(err, scene.ErrClosed) {
		t.Fatalf("closed controller accepted request: %v", err)
	}
}

func TestConfirmControlRejectsPaintRequestsBeforeCallingDomain(t *testing.T) {
	domain := &confirmDomain{snapshot: base.ConfirmSnapshot{Revision: 1}}
	field := base.NewConfirm(base.ConfirmConfig{Control: domain})
	defer field.Close()
	element := field.Element()
	if err := element.SetPaint(func(scene.Frame) { field.Say(true) }); err != nil {
		t.Fatal(err)
	}
	root := newTestRoot(field)
	func() {
		defer func() {
			if recover() == nil {
				t.Error("paint changed controlled answer")
			}
		}()
		root.Draw(grid.NewSurface(8, 1).View())
	}()
	if domain.requests != 0 || domain.snapshot.Value {
		t.Fatal("phase rejection came after a domain mutation")
	}
}
