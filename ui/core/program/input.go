package program

import (
	"context"
	"errors"
	"runtime"
	"time"

	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/scene"
)

// ErrInvalidInput rejects host input without an immutable terminal event value.
var ErrInvalidInput = errors.New("program: input source returned an invalid event")

// ErrInputTooLarge ends input instead of retaining a single unbounded event.
var ErrInputTooLarge = errors.New("program: input event exceeds queue byte budget")

const defaultInputEventCapacity = 256

type ownerInput struct {
	event   scene.CapturedInput
	repaint bool
	reply   func(int, int, error)
	bytes   int
}

// The reader captures the written layout before waiting for the UI owner. The
// finite queue applies backpressure without discarding key or gesture edges.
type inputPipe struct {
	events   chan ownerInput
	consumed chan int
	done     chan struct{}
	result   func() error // Transferred to the owner by closing events.
}

func newInputPipe(ctx context.Context, root *scene.Root, source EventSource, events <-chan input.Event, writtenAt func(time.Time) (uint64, bool), cfg Config) *inputPipe {
	count, bytes := cfg.InputEvents, cfg.InputBytes
	if count == 0 {
		count = defaultInputEventCapacity
	}
	if bytes == 0 {
		bytes = input.MaxDecodedEventBytes
	}
	p := &inputPipe{events: make(chan ownerInput, count), consumed: make(chan int, count+1), done: make(chan struct{})}
	var repaints <-chan func(int, int, error)
	if source, ok := source.(RepaintSource); ok {
		repaints = source.Repaints()
	}
	go p.read(ctx, root, source, events, repaints, writtenAt, bytes)
	return p
}

func (p *inputPipe) read(ctx context.Context, root *scene.Root, source EventSource, events <-chan input.Event, repaints <-chan func(int, int, error), writtenAt func(time.Time) (uint64, bool), limit int) {
	defer close(p.done)
	defer close(p.events)
	completed := false
	defer func() {
		if completed {
			return
		}
		// A host's completion query runs on the reader, but its abnormal exit
		// must unwind the Run owner and its terminal cleanup, not this worker.
		if cause := recover(); cause != nil {
			p.result = func() error { panic(cause) }
		} else {
			p.result = func() error { runtime.Goexit(); return nil }
		}
	}()
	p.receive(ctx, root, source, events, repaints, writtenAt, limit)
	completed = true
}

func (p *inputPipe) receive(ctx context.Context, root *scene.Root, source EventSource, events <-chan input.Event, repaints <-chan func(int, int, error), writtenAt func(time.Time) (uint64, bool), limit int) {
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
				p.result = source.Err
				return
			}
			captured := root.CaptureInput(event, writtenAt)
			item = ownerInput{event: captured, bytes: input.ByteSize(captured.Input())}
		}
		switch {
		case item.bytes == 0:
			p.result = func() error { return ErrInvalidInput }
			return
		case item.bytes > limit:
			p.result = func() error { return ErrInputTooLarge }
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

// An external source's terminal callback belongs to the Run caller, where a
// panic or goroutine exit unwinds normal program and terminal cleanup.
func (p *inputPipe) err() error {
	if p.result == nil {
		return nil
	}
	return p.result()
}
