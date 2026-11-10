package main

import (
	"context"
	"errors"

	"github.com/Tangerg/oolong/ui/core/program"
	"github.com/Tangerg/oolong/ui/core/publication"
	"github.com/Tangerg/oolong/ui/core/text"
)

// outputPump is the worker half of a run. It holds no interface state: the
// decoder and the publisher belong to the UI owner, so feed, flush and offer
// are called only inside owner.Invoke.
type outputPump struct {
	command  []string
	owner    program.Dispatcher
	progress <-chan struct{}
	feed     func(chunk string) ([]text.Line, error)
	flush    func() ([]text.Line, error)
	offer    func(lines []text.Line) (rest []text.Line, err error)
}

// Process cancellation must not revoke publication of the already accepted
// prefix. Delivery keeps the caller's lifetime while the process is stopped.
func (p *outputPump) run(ctx context.Context) error {
	processContext, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	output := &commandWriter{write: func(data []byte) error {
		err := p.deliver(ctx, func() ([]text.Line, error) {
			lines, decodeErr := p.feed(string(data))
			if decodeErr != nil {
				cancel(decodeErr)
			}
			return lines, decodeErr
		})
		if err != nil {
			cancel(err)
		}
		return err
	}}
	result := runCommand(processContext, p.command, output)
	if processContext.Err() != nil {
		return result
	}
	return errors.Join(result, p.deliver(ctx, p.flush))
}

// deliver decodes on the UI owner and offers the lines it produced. The worker
// retains at most one command read while publication is full; an accepted
// prefix is never replayed, and a retry waits for a capacity notification.
func (p *outputPump) deliver(ctx context.Context, decode func() ([]text.Line, error)) error {
	var lines []text.Line
	var decodeErr error
	for {
		err := p.owner.Invoke(ctx, func() error {
			if decode != nil {
				lines, decodeErr = decode()
				decode = nil
			}
			var err error
			lines, err = p.offer(lines)
			return err
		})
		if !errors.Is(err, publication.ErrCapacity) {
			return errors.Join(decodeErr, err)
		}
		select {
		case <-p.progress:
		case <-ctx.Done():
			return errors.Join(decodeErr, context.Cause(ctx))
		}
	}
}

type commandWriter struct{ write func([]byte) error }

func (w *commandWriter) Write(data []byte) (int, error) {
	if err := w.write(data); err != nil {
		return 0, err
	}
	return len(data), nil
}
