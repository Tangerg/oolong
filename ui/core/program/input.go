package program

import (
	"context"
	"errors"

	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/scene"
)

// ErrInvalidInput rejects host input without an immutable terminal event value.
var ErrInvalidInput = errors.New("program: input source returned an invalid event")

// ErrInputTooLarge ends input instead of retaining a single unbounded event.
var ErrInputTooLarge = errors.New("program: input event exceeds queue byte budget")

type ownerInput struct {
	event   scene.CapturedInput
	repaint bool
	reply   func(error)
	bytes   int
}

// The reader captures the written layout before waiting for the UI owner. The
// finite queue applies backpressure without discarding key or gesture edges.
type inputPipe struct {
	events   chan ownerInput
	consumed chan int
	done     chan struct{}
	err      error // Published by closing events.
}

func newInputPipe(ctx context.Context, root *scene.Root, source EventSource, events <-chan input.Event, cfg Config) *inputPipe {
	count, bytes := cfg.InputEvents, cfg.InputBytes
	if count == 0 {
		count = 256
	}
	if bytes == 0 {
		bytes = 4 << 20
	}
	p := &inputPipe{events: make(chan ownerInput, count), consumed: make(chan int, count+1), done: make(chan struct{})}
	var repaints <-chan func(error)
	if source, ok := source.(RepaintSource); ok {
		repaints = source.Repaints()
	}
	go p.read(ctx, root, source, events, repaints, bytes)
	return p
}

func (p *inputPipe) read(ctx context.Context, root *scene.Root, source EventSource, events <-chan input.Event, repaints <-chan func(error), limit int) {
	defer close(p.done)
	defer close(p.events)
	pending := 0
	for {
		var item ownerInput
		select {
		case <-ctx.Done():
			return
		case n := <-p.consumed:
			pending -= n
			continue
		case reply, ok := <-repaints:
			if !ok {
				repaints = nil
				continue
			}
			item = ownerInput{repaint: true, reply: reply, bytes: 1}
		case event, ok := <-events:
			if !ok {
				p.err = source.Err()
				return
			}
			captured := root.CaptureInput(event)
			item = ownerInput{event: captured, bytes: input.ByteSize(captured.Input())}
		}
		switch {
		case item.bytes == 0:
			p.err = ErrInvalidInput
			return
		case item.bytes > limit:
			p.err = ErrInputTooLarge
			return
		}
		// The owner may be returning credit while the event queue is full.
		// Keep receiving credit here so neither bounded channel waits on the other.
		sent := false
		for !sent {
			var output chan<- ownerInput
			if item.bytes <= limit-pending {
				output = p.events
			}
			select {
			case <-ctx.Done():
				return
			case n := <-p.consumed:
				pending -= n
			case output <- item:
				pending += item.bytes
				sent = true
			}
		}
	}
}
