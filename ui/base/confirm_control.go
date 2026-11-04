package base

import (
	"context"
	"errors"

	"github.com/Tangerg/oolong/ui/core/scene"
)

// ErrConfirmStale rejects a request made against a superseded domain revision.
var ErrConfirmStale = errors.New("base: confirm request revision is stale")

// ErrConfirmPending reports a spoken reply whose domain change has started but has not completed.
var ErrConfirmPending = errors.New("base: confirmation is awaiting domain publication")

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
		return readProjection(c.Element(), c.control.Snapshot)
	}
	return ConfirmSnapshot{Value: c.answer.get(), Revision: c.answer.revision}
}

// Request applies the same semantic change used by keys, pointer input and AX.
// External owners may return Started while performing asynchronous work; the
// displayed answer changes only when that owner publishes its next version.
func (c *Confirm) Request(ctx context.Context, request ConfirmRequest) (scene.ActionResult[ConfirmSnapshot], error) {
	return c.request(ctx, request, false)
}

func (c *Confirm) request(ctx context.Context, request ConfirmRequest, recordError bool) (scene.ActionResult[ConfirmSnapshot], error) {
	requireMutable(c.element)
	c.requestDepth++
	defer func() {
		c.requestDepth--
		if c.requestDepth == 0 && c.pendingChange {
			c.invalidate()
		}
	}()
	var turn uint64
	if recordError {
		turn = c.feedback.begin()
	}
	result, err := c.applyRequest(ctx, request)
	if recordError {
		c.pendingChange = c.feedback.finish(turn, err) || c.pendingChange
	}
	c.pendingChange = c.pendingChange || result.State != scene.Unchanged
	return result, err
}

func (c *Confirm) applyRequest(ctx context.Context, request ConfirmRequest) (scene.ActionResult[ConfirmSnapshot], error) {
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
	return scene.ActionResult[ConfirmSnapshot]{State: scene.Applied, Value: c.Snapshot()}, nil
}

func (c *Confirm) invalidate() {
	requireMutable(c.element)
	// A synchronous domain publication must wait for the request's UI feedback.
	if c.requestDepth != 0 {
		c.pendingChange = true
		return
	}
	c.pendingChange = false
	c.element.Invalidate()
}

// ChangeError is the last failure from a keyboard, pointer, Say or Reply request.
// Typed programmatic and AX requests return errors directly.
func (c *Confirm) ChangeError() error { return c.feedback.err }

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
		defer c.unsubscribe()
		c.unsubscribe = nil
	}
	c.invalidate()
}
