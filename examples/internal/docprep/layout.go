package docprep

import (
	"context"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/program"
)

// The immutable projection pins one width while publication captures it. A
// concurrent cache replacement cannot turn a ready check into UI-side layout.
type widthPreparer interface {
	PrepareWidth(ctx context.Context, width int) error
	PreparedLayout(width int) (grid.Drawable, bool)
}

func prepareWidth(ctx context.Context, body grid.Drawable, width int) error {
	if preparer, ok := body.(widthPreparer); ok {
		return preparer.PrepareWidth(ctx, width)
	}
	return context.Cause(ctx)
}

func preparedLayout(body grid.Drawable, width int) (grid.Drawable, bool) {
	if preparer, ok := body.(widthPreparer); ok {
		return preparer.PreparedLayout(width)
	}
	return body, true
}

// Ready is a frame dependency, not a scene geometry owner. Its width comes from
// the compositor, and the caller enumerates the transcript's retained bodies.
// While a worker changes retained layout caches, no frame reads those caches.
func (q *Queue) Ready(width int, transcript *base.Transcript) (bool, error) {
	if q.closed || width <= 0 {
		return false, program.ErrStopped
	}
	q.releaseRevoked()
	var pending []grid.Drawable
	for offset := range transcript.Len() {
		body := transcript.Block(transcript.FirstBlock() + base.BlockID(offset))
		if _, ready := preparedLayout(body, width); !ready {
			pending = append(pending, body)
		}
	}
	if len(pending) > 0 {
		if q.layoutFailure.matches(transcript, width) {
			return false, q.layoutFailure.err
		}
		select {
		case <-q.scope.Context().Done():
			return false, nil
		default:
		}
		if active := q.activeLayout(); active != nil && active.transcript == transcript && active.width == width && !q.active.discard {
			return false, nil
		}
		if q.layout != nil {
			q.layout.release()
		}
		q.layout = &layoutRequest{transcript: transcript, revision: transcript.ContentRevision(), bodies: pending, width: width}
		q.start()
		if q.layoutFailure.matches(transcript, width) {
			return false, q.layoutFailure.err
		}
		return false, nil
	}
	q.layoutFailure = nil
	return q.activeLayout() == nil, nil
}

type layoutRequest struct {
	transcript *base.Transcript
	revision   uint64
	bodies     []grid.Drawable
	width      int
}

// A failed content revision/width attempt cannot recover by polling. Clear alone
// must not create an error/retry loop; a new content revision can try again.
type layoutFailure struct {
	transcript *base.Transcript
	revision   uint64
	width      int
	err        error
}

func (failure *layoutFailure) matches(transcript *base.Transcript, width int) bool {
	return failure != nil && failure.transcript == transcript && failure.revision == transcript.ContentRevision() && failure.width == width
}

func (q *Queue) activeLayout() *layoutRequest {
	if q.active == nil {
		return nil
	}
	layout, _ := q.active.request.(*layoutRequest)
	return layout
}

func (request *layoutRequest) start(q *Queue, active *execution) (*program.Task, error) {
	bodies, width := request.bodies, request.width
	return q.launch(active, func(ctx context.Context) (struct{}, error) {
		for _, body := range bodies {
			if err := prepareWidth(ctx, body, width); err != nil {
				return struct{}{}, err
			}
		}
		return struct{}{}, nil
	}, func(_ struct{}, err error) bool {
		if err != nil {
			request.reject(q, err)
			return false
		}
		return true
	})
}

func (request *layoutRequest) reject(q *Queue, err error) {
	q.layoutFailure = &layoutFailure{transcript: request.transcript, revision: request.revision, width: request.width, err: err}
}

func (request *layoutRequest) release() {
	request.transcript, request.bodies = nil, nil
}
