// Package automation observes and invokes the same scene used by human input.
// It owns query intent and scheduling, never a second UI model.
package automation

import (
	"context"
	"errors"

	"github.com/Tangerg/oolong/ui/core/scene"
)

// Owner supplies the interface owner's read and action admission boundaries.
// Inspect must not request a new frame merely because a pure query ran.
type Owner interface {
	Inspect(ctx context.Context, inspect func() error) error
	Invoke(ctx context.Context, invoke func() error) error
	Done() <-chan struct{}
}

// DriverConfig connects one scene to its interface-owner dispatcher.
type DriverConfig struct {
	Root  *scene.Root
	Owner Owner
}

// Driver serializes local automation through the same owner used by physical input. It owns no UI state.
type Driver struct{ config DriverConfig }

// Driver errors distinguish invalid configuration from an owner that has stopped.
var (
	ErrInvalid = errors.New("automation: root and owner are required")
	ErrStopped = errors.New("automation: owner stopped")
)

// NewDriver creates a local driver. Operations report ErrInvalid for a missing root or owner.
func NewDriver(cfg DriverConfig) *Driver { return &Driver{config: cfg} }

func (d *Driver) valid() error {
	if d == nil || d.config.Root == nil || d.config.Owner == nil {
		return ErrInvalid
	}
	return nil
}

// Observe obtains a read-only semantic snapshot on the owner sequence.
func (d *Driver) Observe(ctx context.Context, query scene.Query) (scene.Observation, error) {
	if err := d.valid(); err != nil {
		return scene.Observation{}, err
	}
	var observation scene.Observation
	err := d.config.Owner.Inspect(ctx, func() error {
		var err error
		observation, err = d.config.Root.Observe(query)
		return err
	})
	return observation, err
}

// Find returns the unique match in a complete bounded observation.
func (d *Driver) Find(ctx context.Context, query scene.Query, match scene.Match) (scene.Target, error) {
	observation, err := d.Observe(ctx, query)
	if err != nil {
		return scene.Target{}, err
	}
	return observation.FindUnique(match)
}

// Invoke executes an action against one exact attachment on the owner sequence.
func (d *Driver) Invoke(ctx context.Context, target scene.Target, action scene.ActionID, params any) (scene.Invocation, error) {
	if err := d.valid(); err != nil {
		return scene.Invocation{}, err
	}
	var result scene.Invocation
	err := d.config.Owner.Invoke(ctx, func() error {
		var err error
		result, err = d.config.Root.Invoke(ctx, target, action, params)
		return err
	})
	return result, err
}

// Locator is intent, not a retained target. Each invocation resolves exactly one
// matching current attachment and executes in that same owner turn.
type Locator struct {
	driver *Driver
	query  scene.Query
	match  scene.Match
}

// Locate creates a query that resolves again when invoked, rather than retaining an attachment.
func (d *Driver) Locate(query scene.Query, match scene.Match) Locator {
	return Locator{driver: d, query: query, match: match}
}

// Invoke resolves one unique target and executes its action in the same owner turn.
func (l Locator) Invoke(ctx context.Context, action scene.ActionID, params any) (scene.Invocation, error) {
	if err := l.driver.valid(); err != nil {
		return scene.Invocation{}, err
	}
	var result scene.Invocation
	err := l.driver.config.Owner.Invoke(ctx, func() error {
		observation, err := l.driver.config.Root.Observe(l.query)
		if err != nil {
			return err
		}
		target, err := observation.FindUnique(l.match)
		if err != nil {
			return err
		}
		result, err = l.driver.config.Root.Invoke(ctx, target, action, params)
		return err
	})
	return result, err
}

// Wait observes and subscribes in one owner turn, so a transition cannot occur
// between the predicate's snapshot and registration. It waits only for the stated
// condition, not for every task an application might create in the future.
func (d *Driver) Wait(ctx context.Context, query scene.Query, ready func(scene.Observation) bool) (scene.Observation, error) {
	if err := d.valid(); err != nil {
		return scene.Observation{}, err
	}
	if ready == nil {
		return scene.Observation{}, ErrInvalid
	}
	for {
		var observation scene.Observation
		var changed <-chan struct{}
		err := d.config.Owner.Inspect(ctx, func() error {
			var err error
			observation, err = d.config.Root.Observe(query)
			changed = d.config.Root.Changes()
			return err
		})
		if err != nil {
			return scene.Observation{}, err
		}
		if ready(observation) {
			return observation, nil
		}
		select {
		case <-changed:
		case <-ctx.Done():
			return scene.Observation{}, context.Cause(ctx)
		case <-d.config.Owner.Done():
			return scene.Observation{}, ErrStopped
		}
	}
}
