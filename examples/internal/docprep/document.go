package docprep

import (
	"context"
	"errors"

	"github.com/Tangerg/oolong/ui/core/content"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/program"
)

type documentRequest struct {
	document *content.StreamDocument
	source   content.SourceSnapshot
	render   content.Renderer
	finished func(error)
}

func (request *documentRequest) start(q *Queue, active *execution) (*program.Task, error) {
	source, render := request.source, request.render
	width, _ := q.runtime.Size()
	return q.launch(active, func(ctx context.Context) (content.Prepared, error) {
		prepared := source.Prepare(ctx, func(ctx context.Context, source string) (grid.Drawable, error) {
			if render == nil {
				return nil, content.ErrUnknownFormat
			}
			body, err := render(ctx, source)
			if grid.IsNilDrawable(body) {
				return body, err
			}
			if layoutErr := prepareWidth(ctx, body, width); layoutErr != nil {
				return nil, errors.Join(err, layoutErr)
			}
			return body, err
		})
		return prepared, nil
	}, func(prepared content.Prepared, err error) bool {
		request.accept(q, prepared, err)
		return false
	})
}

func (request *documentRequest) reject(q *Queue, err error) {
	if !request.accept(q, content.Prepared{}, err) {
		q.report(err)
	}
}

func (request *documentRequest) accept(q *Queue, prepared content.Prepared, workErr error) bool {
	if request.document == nil || request.source.Revision() != request.document.Current().Revision {
		return false
	}
	if workErr != nil {
		prepared = request.source.Prepare(context.Background(), func(context.Context, string) (grid.Drawable, error) {
			return nil, workErr
		})
	}
	if err := request.document.Accept(prepared); err != nil {
		q.report(err)
		return true
	}
	if request.source.Final() && request.finished != nil {
		request.finished(finalError(request.document.Current()))
		return true
	}
	return false
}

func (request *documentRequest) release() {
	request.document, request.finished = nil, nil
	request.source, request.render = content.SourceSnapshot{}, nil
}

func finalError(change content.DocumentChange) error {
	err := change.SourceError
	if change.PrepareError != nil {
		err = errors.Join(err, change.PrepareError)
	}
	return err
}
