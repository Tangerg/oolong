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
	pressure chan chan bool
	done     chan struct{}
	result   func() error // Transferred to the owner by closing events.
	// Only the reader advances the byte budget. The owner asks it to recheck
	// admission while a repaint barrier prevents normal queue consumption.
	pendingBytes int
	byteLimit    int
}

func newInputPipe(ctx context.Context, root *scene.Root, source EventSource, events <-chan input.Event, writtenAt func(time.Time) (uint64, bool), cfg Config) *inputPipe {
	count, bytes := cfg.InputEvents, cfg.InputBytes
	if count == 0 {
		count = defaultInputEventCapacity
	}
	if bytes == 0 {
		bytes = input.MaxDecodedEventBytes
	}
	p := &inputPipe{
		events: make(chan ownerInput, count), consumed: make(chan int, count+1),
		pressure: make(chan chan bool), done: make(chan struct{}), byteLimit: bytes,
	}
	var repaints <-chan func(int, int, error)
	if source, ok := source.(RepaintSource); ok {
		repaints = source.Repaints()
	}
	go p.read(ctx, root, source, events, repaints, writtenAt)
	return p
}

func (p *inputPipe) read(ctx context.Context, root *scene.Root, source EventSource, events <-chan input.Event, repaints <-chan func(int, int, error), writtenAt func(time.Time) (uint64, bool)) {
	defer close(p.done)
	defer close(p.events)
	completed := false
	defer func() {
		if completed {
			return
		}
		// A host's input callbacks run on the reader, but their abnormal exit
		// must unwind the Run owner and its terminal cleanup, not this worker.
		if cause := recover(); cause != nil {
			p.result = func() error { panic(cause) }
		} else {
			p.result = func() error { runtime.Goexit(); return nil }
		}
	}()
	p.receive(ctx, root, source, events, repaints, writtenAt)
	completed = true
}

func (p *inputPipe) receive(ctx context.Context, root *scene.Root, source EventSource, events <-chan input.Event, repaints <-chan func(int, int, error), writtenAt func(time.Time) (uint64, bool)) {
	pointers, release, err := bindPointerInput(root, source)
	if release != nil {
		defer release()
	}
	if err != nil {
		p.result = func() error { return err }
		return
	}
	for {
		var item ownerInput
		select {
		case <-ctx.Done():
			return
		case n := <-p.consumed:
			p.pendingBytes -= n
			continue
		case reply, ok := <-repaints:
			if !ok {
				repaints = nil
				continue
			}
			item = ownerInput{repaint: true, reply: reply, bytes: 1}
		case captured, ok := <-pointers:
			if !ok {
				pointers = nil
				continue
			}
			item = ownerInput{event: captured, bytes: input.ByteSize(captured.Input())}
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
		case item.bytes > p.byteLimit:
			p.result = func() error { return ErrInputTooLarge }
			return
		}
		if !p.deliver(ctx, item) {
			return
		}
	}
}

func (p *inputPipe) deliver(ctx context.Context, item ownerInput) bool {
	sent := false
	for !sent {
		select {
		case <-ctx.Done():
			return false
		case n := <-p.consumed:
			p.pendingBytes -= n
		case p.outputFor(item) <- item:
			sent = true
		case reply := <-p.pressure:
			// The owner is waiting for this answer, so no new credit can arrive.
			// Collect its preceding acknowledgements before deciding whether the
			// barrier must yield; a queued pressure notification could be stale.
			p.collectCredits()
			select {
			case p.outputFor(item) <- item:
				sent = true
			default:
			}
			reply <- !sent
		}
	}
	p.pendingBytes += item.bytes
	return true
}

func (p *inputPipe) outputFor(item ownerInput) chan<- ownerInput {
	if item.bytes > p.byteLimit-p.pendingBytes {
		return nil
	}
	return p.events
}

func (p *inputPipe) collectCredits() {
	for {
		select {
		case n := <-p.consumed:
			p.pendingBytes -= n
		default:
			return
		}
	}
}

// Closing events transfers the terminal result to the Run owner. Claim it before
// invoking external code so EOF and shutdown cannot settle or unwind it twice.
func (p *inputPipe) takeResult() error {
	result := p.result
	p.result = nil
	if result == nil {
		return nil
	}
	return result()
}
