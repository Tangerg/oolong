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
	feed     func(chunk string) []text.Line
	flush    func() []text.Line
	offer    func(lines []text.Line) (rest []text.Line, err error)
}

// run reports why a cancelled run ended through runCommand alone, so the cause
// appears once; a cancelled run publishes nothing more.
func (p *outputPump) run(ctx context.Context) error {
	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	output := &commandWriter{write: func(data []byte) error {
		err := p.deliver(ctx, func() []text.Line { return p.feed(string(data)) })
		if err != nil {
			cancel(err)
		}
		return err
	}}
	result := runCommand(ctx, p.command, output)
	if ctx.Err() != nil {
		return result
	}
	return errors.Join(result, p.deliver(ctx, p.flush))
}

// deliver decodes on the UI owner and offers the lines it produced. The worker
// retains at most one command read while publication is full; an accepted
// prefix is never replayed, and a retry waits for a capacity notification.
func (p *outputPump) deliver(ctx context.Context, decode func() []text.Line) error {
	var lines []text.Line
	for {
		err := p.owner.Invoke(ctx, func() error {
			if decode != nil {
				lines = decode()
				decode = nil
			}
			var err error
			lines, err = p.offer(lines)
			return err
		})
		if !errors.Is(err, publication.ErrCapacity) {
			return err
		}
		select {
		case <-p.progress:
		case <-ctx.Done():
			return context.Cause(ctx)
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
