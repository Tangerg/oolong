// Package docprep admits one actual worker at a time. Cancellation cannot release
// that admission; pending source snapshots coalesce within their document owner.
package docprep

import (
	"context"
	"errors"

	"github.com/Tangerg/oolong/ui/core/content"
	"github.com/Tangerg/oolong/ui/core/program"
)

const maxPendingDocuments = 64

var ErrCapacity = errors.New("docprep: pending document budget exhausted")

type request interface {
	start(queue *Queue, active *execution) (*program.Task, error)
	reject(queue *Queue, err error)
	release()
}

type execution struct {
	request request
	task    *program.Task
	discard bool
}

// Queue belongs to the UI owner. Runtime owns worker outcomes and delivery;
// StreamDocument alone owns source revisions and accepted preparation results.
type Queue struct {
	runtime       *program.Runtime
	scope         *program.Scope
	active        *execution
	documents     []*documentRequest
	layout        *layoutRequest
	layoutFailure *layoutFailure
	after         []func()
	publisher     *Publisher
	failed        func(error)
	closed        bool
	pumping       bool
}

type Config struct {
	Runtime *program.Runtime
	Scope   *program.Scope
	// Failed receives source scheduling failures without a final callback.
	// Finished owns final document outcomes; Ready reports retained layout errors.
	Failed func(error)
}

func New(cfg Config) *Queue {
	if cfg.Runtime == nil || cfg.Scope == nil || cfg.Failed == nil {
		panic("docprep: runtime, scope and failure receiver are required")
	}
	return &Queue{runtime: cfg.Runtime, scope: cfg.Scope, failed: cfg.Failed}
}

func (q *Queue) Prepare(document *content.StreamDocument, render content.Renderer, finished func(error)) error {
	if document == nil {
		return errors.New("docprep: document is nil")
	}
	if err := q.openError(); err != nil {
		return err
	}
	current := document.Current()
	source := document.Snapshot()
	if current.Revision == current.PreparedRevision && (current.Body != nil || current.PrepareError != nil) {
		if source.Final() && finished != nil {
			finished(finalError(current))
		}
		return nil
	}
	next := &documentRequest{document: document, source: source, render: render, finished: finished}
	if q.coalesce(next) {
		return nil
	}
	if len(q.documents) == maxPendingDocuments {
		return ErrCapacity
	}
	q.documents = append(q.documents, next)
	q.start()
	return nil
}

func (q *Queue) coalesce(next *documentRequest) bool {
	if active := q.active; active != nil && !active.discard {
		if request, ok := active.request.(*documentRequest); ok && request.document == next.document {
			if request.source.Revision() == next.source.Revision() {
				if next.finished != nil {
					request.finished = next.finished
				}
				return true
			}
			active.task.Cancel()
		}
	}
	for index, pending := range q.documents {
		if pending.document == next.document {
			if next.finished == nil {
				next.finished = pending.finished
			}
			pending.release()
			q.documents[index] = next
			return true
		}
	}
	return false
}

func (q *Queue) openError() error {
	if q.closed {
		return program.ErrScopeClosed
	}
	return context.Cause(q.scope.Context())
}

func (q *Queue) start() {
	if q.pumping || q.active != nil || q.closed {
		return
	}
	q.pumping = true
	defer func() { q.pumping = false }()
	for !q.closed && q.active == nil {
		next := q.take()
		if next == nil {
			if len(q.after) == 0 {
				return
			}
			fn := q.after[0]
			q.after[0], q.after = nil, q.after[1:]
			fn()
			continue
		}
		active := &execution{request: next}
		q.active = active
		task, err := next.start(q, active)
		if err == nil {
			active.task = task
			return
		}
		q.active = nil
		next.reject(q, err)
		next.release()
	}
}

func (q *Queue) take() request {
	if q.layout != nil {
		next := q.layout
		q.layout = nil
		return next
	}
	if len(q.documents) == 0 {
		return nil
	}
	next := q.documents[0]
	q.documents[0], q.documents = nil, q.documents[1:]
	return next
}

func (q *Queue) launch[T any](active *execution, run func(context.Context) (T, error), apply func(T, error) bool) (*program.Task, error) {
	return q.runtime.Start(q.scope, program.Work[T]{
		Run: run,
		Apply: func(result T, err error) {
			notify := false
			if !q.closed && !active.discard {
				notify = apply(result, err)
			}
			q.active = nil
			active.request.release()
			q.start()
			if q.closed {
				return
			}
			if notify && q.activeLayout() == nil && q.publisher != nil {
				q.publisher.notify()
			}
			q.runtime.Refresh()
		},
	})
}

func (q *Queue) report(err error) {
	if !q.closed {
		q.failed(err)
	}
}

func (q *Queue) releaseRevoked() {
	active := q.active
	if active == nil || active.task == nil {
		return
	}
	select {
	case <-active.task.Done():
		if active.task.Snapshot().Delivery == program.DeliveryRevoked {
			q.Clear()
			q.active = nil
		}
	default:
	}
}

// After waits for actual worker completion while this queue's scope remains live.
// Clear and Close revoke callbacks; scope closure revokes runtime delivery.
func (q *Queue) After(fn func()) error {
	if err := q.openError(); err != nil {
		return err
	}
	if fn == nil {
		return nil
	}
	q.after = append(q.after, fn)
	q.start()
	return nil
}

// Clear revokes result delivery without admitting work over an executing worker.
func (q *Queue) Clear() {
	for _, pending := range q.documents {
		pending.release()
	}
	q.documents, q.after = nil, nil
	if q.layout != nil {
		q.layout.release()
		q.layout = nil
	}
	if q.active != nil {
		q.active.discard = true
		q.active.request.release()
		q.active.task.Cancel()
	}
}

func (q *Queue) Close() {
	if q.closed {
		return
	}
	q.closed = true
	q.Clear()
	if q.publisher != nil {
		q.publisher.close()
	}
	q.active, q.layoutFailure, q.publisher = nil, nil, nil
	q.runtime, q.scope, q.failed = nil, nil, nil
}
