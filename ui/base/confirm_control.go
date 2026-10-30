package base

import (
	"context"
	"errors"

	"github.com/Tangerg/oolong/ui/core/scene"
)

// ErrConfirmStale rejects a request made against a superseded domain revision.
var ErrConfirmStale = errors.New("base: confirm request revision is stale")

// ConfirmSnapshot is an atomic domain value and its monotonically increasing
// revision. A revision changes even when the value changes away and back.
type ConfirmSnapshot struct {
	Value    bool
	Revision uint64
}

// ConfirmRequest asks a domain owner to change the value only if ExpectedRevision
// still matches. The domain performs that check and mutation in the same turn.
type ConfirmRequest struct {
	ExpectedRevision uint64
	Value            bool
}

// ConfirmControl is the narrow protocol for a domain-owned live yes/no value.
// Snapshot returns one immutable version; it must not normalize or mutate. Change
// atomically verifies ExpectedRevision and returns ErrConfirmStale on conflict.
// Subscribe delivers completed domain changes on the UI owner turn and returns an
// idempotent unsubscribe function. It must not invoke the callback while subscribing.
// Background work must deliver through the runtime before invoking subscribers.
// No controller-side writable copy of the domain value is retained.
type ConfirmControl interface {
	Snapshot() ConfirmSnapshot
	Change(ctx context.Context, request ConfirmRequest) (scene.ActionResult[ConfirmSnapshot], error)
	Subscribe(notify func()) func()
}

// Snapshot reads one complete value version from the selected value owner.
func (c *Confirm) Snapshot() ConfirmSnapshot {
	if c.control != nil {
		return c.control.Snapshot()
	}
	return ConfirmSnapshot{Value: c.answer.get(), Revision: c.answer.revision}
}

// Request applies the same semantic change used by keys, pointer input and AX.
// External owners may return Started while performing asynchronous work; the
// displayed answer changes only when that owner publishes its next version.
func (c *Confirm) Request(ctx context.Context, request ConfirmRequest) (scene.ActionResult[ConfirmSnapshot], error) {
	requireMutable(c.element)
	if c.closed {
		return scene.ActionResult[ConfirmSnapshot]{}, scene.ErrClosed
	}
	if err := ctx.Err(); err != nil {
		return scene.ActionResult[ConfirmSnapshot]{}, err
	}
	before := c.Snapshot()
	if request.ExpectedRevision != before.Revision {
		return scene.ActionResult[ConfirmSnapshot]{}, ErrConfirmStale
	}
	if request.Value == before.Value {
		return scene.ActionResult[ConfirmSnapshot]{State: scene.Unchanged, Value: before}, nil
	}
	if c.control != nil {
		return c.control.Change(ctx, request)
	}
	c.answer.set(request.Value)
	c.element.Invalidate()
	return scene.ActionResult[ConfirmSnapshot]{State: scene.Applied, Value: c.Snapshot()}, nil
}

// ChangeError is the last failure from a keyboard, pointer, Say or Reply request.
// Typed programmatic and AX requests return errors directly.
func (c *Confirm) ChangeError() error { return c.changeErr }

// Close releases a live domain subscription and disables further requests. It
// does not dispose the domain owner. Temporary scene detachment does not close the
// subscription: the same controller can be mounted again until explicitly closed.
// It panics if called during scene presentation or observation.
func (c *Confirm) Close() {
	requireMutable(c.element)
	if c.closed {
		return
	}
	c.closed = true
	if c.unsubscribe != nil {
		c.unsubscribe()
		c.unsubscribe = nil
	}
	c.element.Invalidate()
}
