package program

import (
	"context"
	"errors"
	"math"
	"sync"

	"github.com/Tangerg/oolong/ui/core/scene"
)

var (
	// ErrScopeClosed revokes new work and application of later results.
	ErrScopeClosed = errors.New("program: task scope is closed")
	// ErrForeignScope prevents resources crossing runtime ownership boundaries.
	ErrForeignScope = errors.New("program: task scope belongs to another session")
	// ErrTaskAborted settles a worker that exited without returning an outcome.
	ErrTaskAborted = errors.New("program: worker exited without returning")
)

// Scope owns cancellation and delivery eligibility, not the worker's outcome.
// Closing a scope requests cooperative cancellation and revokes UI delivery.
// A Scope must not be copied after construction.
type Scope struct {
	ctx        context.Context //nolint:containedctx // The scope owns cancellation for all resources attached to its lifetime.
	external   context.Context //nolint:containedctx // Admission reads the external lifetime directly; its asynchronous cancellation bridge is only a notification.
	cancel     context.CancelCauseFunc
	queue      *taskQueue
	slot       *reservation
	attachment scene.Attachment
	closeOnce  sync.Once
	stopParent func() bool
}

// ScopeConfig combines an optional external lifetime with attachment lifetime.
// Either ending revokes result delivery; the runtime remains the parent owner.
type ScopeConfig struct {
	Context    context.Context //nolint:containedctx // This optional lifetime parent is retained by construction, not by an individual request.
	Attachment scene.Attachment
}

// NewScope reserves bounded lifetime capacity before starting its cancellation
// watcher. A detached attachment is rejected before any work is accepted.
func (r *Runtime) NewScope(cfg ScopeConfig) (*Scope, error) {
	p := r.owner()
	if p == nil || p.ctx == nil {
		return nil, ErrStopped
	}
	target := cfg.Attachment.Target()
	if !target.IsZero() && (p.root == nil || !p.root.Valid(target)) {
		return nil, ErrScopeClosed
	}
	if cfg.Context != nil && cfg.Context.Err() != nil {
		return nil, context.Cause(cfg.Context)
	}
	slot, err := p.tasks.reserve()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancelCause(p.ctx)
	scope := &Scope{ctx: ctx, external: cfg.Context, cancel: cancel, queue: p.tasks, slot: slot, attachment: cfg.Attachment}
	if cfg.Context != nil {
		scope.stopParent = context.AfterFunc(cfg.Context, func() { cancel(context.Cause(cfg.Context)) })
	}
	go func() {
		if target.IsZero() {
			<-ctx.Done()
		} else {
			select {
			case <-ctx.Done():
			case <-cfg.Attachment.Done():
				cancel(ErrScopeClosed)
			}
		}
		scope.Close()
	}()
	return scope, nil
}

// Context shares cancellation with accepted work without granting UI access.
// A nil or unconstructed scope is a programming error and panics.
func (s *Scope) Context() context.Context {
	if s == nil || s.ctx == nil {
		panic("program: unconstructed task scope")
	}
	return s.ctx
}

// Close requests cancellation and revokes application of later results. It does
// not report a worker as finished or wait for noncooperative external code.
func (s *Scope) Close() {
	if s == nil {
		return
	}
	s.closeOnce.Do(func() {
		s.cancel(ErrScopeClosed)
		if s.stopParent != nil {
			s.stopParent()
		}
		s.slot.close()
	})
}

func (s *Scope) eligible(p *program) bool {
	if s == nil || s.queue != p.tasks || s.ctx.Err() != nil {
		return false
	}
	if s.external != nil && s.external.Err() != nil {
		return false
	}
	target := s.attachment.Target()
	return target.IsZero() || (p.root != nil && p.root.Valid(target))
}

// TaskState records the worker outcome, independently of cancellation requests.
type TaskState uint8

// Worker outcomes change once, when Run returns or exits abnormally.
const (
	TaskRunning TaskState = iota
	TaskSucceeded
	TaskFailed
	TaskCancelled
)

// DeliveryState records whether a completed result reached its current UI owner.
type DeliveryState uint8

// Delivery has its own terminal state because successful work can outlive a view.
const (
	DeliveryPending DeliveryState = iota
	DeliveryApplied
	DeliveryRevoked
)

// TaskStatus is a detached observation of execution, cancellation and delivery.
type TaskStatus struct {
	ID              uint64
	State           TaskState
	Delivery        DeliveryState
	CancelRequested bool
	Err             error
}

// Task owns execution status. Results move from Run to Apply; the handle does
// not retain a second writable copy of an application result.
// A Task must not be copied after construction.
type Task struct {
	mu            sync.Mutex
	status        TaskStatus
	cancel        context.CancelCauseFunc
	done, settled chan struct{}
	claimed       bool
	stopDelivery  func() bool
}

// Work transfers a result from Run on a worker to Apply on the UI owner. Run
// must cooperate with cancellation; Apply must not block and may be revoked.
type Work[T any] struct {
	Run   func(context.Context) (T, error)
	Apply func(T, error)
}

// Start reserves completion capacity before launching work. Ordinary Post
// saturation cannot lose the result or leave an accepted task permanently busy.
func (r *Runtime) Start[T any](scope *Scope, work Work[T]) (*Task, error) {
	p := r.owner()
	if p == nil {
		return nil, ErrStopped
	}
	if scope == nil || scope.queue != p.tasks {
		return nil, ErrForeignScope
	}
	if !scope.eligible(p) {
		return nil, ErrScopeClosed
	}
	if work.Run == nil {
		return nil, errors.New("program: work requires Run")
	}
	slot, err := p.tasks.reserve()
	if err != nil {
		return nil, err
	}
	if p.nextTask == math.MaxUint64 {
		slot.close()
		return nil, errors.New("program: task identity exhausted")
	}
	p.nextTask++
	ctx, cancel := context.WithCancelCause(scope.ctx)
	task := &Task{
		status: TaskStatus{ID: p.nextTask}, cancel: cancel,
		done: make(chan struct{}), settled: make(chan struct{}),
	}
	task.stopDelivery = context.AfterFunc(scope.ctx, task.revoke)
	go task.run(ctx, scope, p, slot, work)
	return task, nil
}

func (t *Task) run[T any](ctx context.Context, scope *Scope, p *program, slot *reservation, work Work[T]) {
	stopCancellation := context.AfterFunc(ctx, t.noteCancellation)
	var value T
	err := ErrTaskAborted
	defer func() {
		stopCancellation()
		if ctx.Err() != nil {
			t.noteCancellation()
		}
		t.complete(err)
		t.cancel(nil)
		if t.Snapshot().Delivery != DeliveryPending {
			slot.close()
			return
		}
		if !slot.post(func() bool {
			defer slot.close()
			applied := false
			defer func() { t.settle(applied) }()
			if scope.eligible(p) && work.Apply != nil && t.claimDelivery() {
				work.Apply(value, err)
				applied = true
			}
			return applied
		}) {
			slot.close()
			t.settle(false)
		} else {
			// A stopped owner can discard a queued completion. The task still settles
			// its delivery fact without invoking application code on this goroutine.
			select {
			case <-t.settled:
			case <-p.tasks.done:
				t.settle(false)
				slot.close()
			}
		}
	}()
	value, err = work.Run(ctx)
}

// Snapshot is safe from any goroutine and never grants access to the result.
func (t *Task) Snapshot() TaskStatus {
	if t == nil {
		return TaskStatus{State: TaskFailed, Delivery: DeliveryRevoked, Err: ErrStopped}
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.status
}

// Cancel requests cooperative cancellation once. A worker may still succeed;
// closing its Scope is the operation that independently revokes UI delivery.
func (t *Task) Cancel() bool {
	if t == nil {
		return false
	}
	t.mu.Lock()
	if t.status.State != TaskRunning || t.status.CancelRequested {
		t.mu.Unlock()
		return false
	}
	t.status.CancelRequested = true
	t.mu.Unlock()
	t.cancel(context.Canceled)
	return true
}

func (t *Task) noteCancellation() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.status.State == TaskRunning {
		t.status.CancelRequested = true
	}
}

// Done closes on the worker outcome, before application or frame publication.
func (t *Task) Done() <-chan struct{} {
	if t == nil {
		return closedSignal
	}
	return t.done
}

// Settled closes when delivery is applied or revoked, even if a revoked worker
// is still running. It does not promise that
// a resulting frame has reached the terminal.
func (t *Task) Settled() <-chan struct{} {
	if t == nil {
		return closedSignal
	}
	return t.settled
}

func (t *Task) complete(err error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.status.Err = err
	switch {
	case err == nil:
		t.status.State = TaskSucceeded
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded), errors.Is(err, ErrScopeClosed):
		t.status.State = TaskCancelled
	default:
		t.status.State = TaskFailed
	}
	close(t.done)
}

func (t *Task) claimDelivery() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.status.Delivery != DeliveryPending {
		return false
	}
	t.claimed = true
	return true
}

func (t *Task) revoke() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.claimed || t.status.Delivery != DeliveryPending {
		return
	}
	t.status.Delivery = DeliveryRevoked
	close(t.settled)
}

func (t *Task) settle(applied bool) {
	if t.stopDelivery != nil {
		t.stopDelivery()
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.status.Delivery != DeliveryPending {
		return
	}
	t.status.Delivery = DeliveryRevoked
	if applied {
		t.status.Delivery = DeliveryApplied
	}
	close(t.settled)
}
