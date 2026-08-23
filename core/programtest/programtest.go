// Package programtest drives a program without opening a terminal.
//
// Host implements only [program.Host]'s transport boundary: ordered events in, frames
// out, and an opening cell size. Optional terminal capabilities are deliberately not
// present. A test that needs one can embed *Host in its own type and implement exactly
// that capability, so absence and partial support remain testable rather than being
// hidden by an all-powerful fake.
//
// Frames are terminal escape streams, not a simulated screen. Repaint requests a full
// frame before an assertion, which keeps this package small and keeps renderer tests
// honest about the bytes a host actually receives.
package programtest

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/internal/fifo"
	"github.com/Tangerg/oolong/core/program"
	"github.com/Tangerg/oolong/core/term"
)

const assertionTimeout = 5 * time.Second

// Host is an in-memory program host for tests.
//
// Every method is safe to call from the test goroutine while [program.Run] owns the
// program goroutine. New arranges cleanup; Close is also available when a test wants
// to settle the frame writer earlier. The zero Host is inert and refuses events. A
// Host must not be copied after first use: its event stream, frame writer and captured
// output are one test session.
type Host struct {
	input  *eventSource
	writer *term.Writer
	out    *sink

	sizeMu sync.Mutex
	w, h   int
	closed atomic.Bool
}

// Config is the complete initial state of a test [Host]. Width and Height must be
// positive terminal-cell dimensions.
type Config struct {
	Width  int
	Height int
}

// New returns a host of the configured positive cell size.
//
// t owns the host. Its cleanup closes the asynchronous frame writer after the test
// context has been cancelled, so a failed test cannot leave a goroutine behind.
func New(tb testing.TB, config Config) *Host {
	tb.Helper()
	if err := program.ValidateSize(config.Width, config.Height); err != nil {
		tb.Fatalf("programtest: %v", err)
	}
	out := newSink()
	host := &Host{
		input:  newEventSource(),
		writer: term.NewWriter(out),
		out:    out,
		w:      config.Width,
		h:      config.Height,
	}
	tb.Cleanup(func() {
		if err := host.Close(); err != nil {
			tb.Errorf("programtest: close host: %v", err)
		}
	})
	return host
}

// Input returns the host's clean-ending input stream.
func (h *Host) Input() program.EventSource { return h }

// Events is the ordered input channel.
func (h *Host) Events() <-chan input.Event {
	if h == nil || h.input == nil {
		return nil
	}
	return h.input.events
}

// Err reports a clean input end. Tests normally stop a program through their
// context or the program's own quit action.
func (h *Host) Err() error { return nil }

// Writer is where the program queues terminal frames.
func (h *Host) Writer() program.FrameWriter {
	if h == nil || h.writer == nil {
		return nil
	}
	return h.writer
}

// Size returns the opening size passed to New.
func (h *Host) Size() (width, height int, err error) {
	if h == nil {
		return 0, 0, nil
	}
	h.sizeMu.Lock()
	defer h.sizeMu.Unlock()
	return h.w, h.h, nil
}

// Send queues an event for the program and reports whether the host was open.
// It does not wait for the program to receive earlier events.
func (h *Host) Send(event input.Event) bool {
	if h == nil || h.input == nil || h.closed.Load() {
		return false
	}
	return h.input.post(event)
}

// Type sends text one character key at a time.
func (h *Host) Type(text string) bool {
	if h == nil || h.input == nil || h.closed.Load() {
		return false
	}
	for _, r := range text {
		if !h.Send(input.Key{Code: input.Character, Rune: r}) {
			return false
		}
	}
	return true
}

// Press sends a non-character key.
func (h *Host) Press(code input.Code) bool {
	return h.Send(input.Key{Code: code})
}

// Resize changes the reported size and sends the corresponding event.
// Dimensions outside [program.ValidateSize] are rejected.
func (h *Host) Resize(width, height int) bool {
	if program.ValidateSize(width, height) != nil || h == nil || h.input == nil || h.closed.Load() {
		return false
	}
	h.sizeMu.Lock()
	h.w, h.h = width, height
	h.sizeMu.Unlock()
	return h.Send(input.Resize{Width: width, Height: height})
}

// Repaint asks the program for a full frame at its current size.
func (h *Host) Repaint() bool {
	if h == nil || h.input == nil {
		return false
	}
	width, height, _ := h.Size()
	return h.Send(input.Resize{Width: width, Height: height})
}

// Frames returns every byte written so far as a string.
func (h *Host) Frames() string {
	if h == nil || h.out == nil {
		return ""
	}
	return h.out.String()
}

// Frame returns the most recent frame write.
//
// Ordinary frames are diffs. Call Repaint before an assertion that needs the whole
// screen, or use Shows and Hides, which do that automatically.
func (h *Host) Frame() string {
	if h == nil || h.out == nil {
		return ""
	}
	return h.out.Last()
}

// Until waits for output activity until cond becomes true, failing the test with
// the last frame after a bounded wait.
//
// cond must inspect concurrency-safe state: Host output methods are safe, as is
// application-owned state protected by its own synchronization. The wait is driven
// by frame writes rather than a polling sleep.
func (h *Host) Until(tb testing.TB, what string, cond func() bool) {
	tb.Helper()
	if h == nil || h.out == nil || cond == nil {
		tb.Fatal("programtest: cannot wait with an uninitialized host or nil condition")
	}
	ctx, cancel := context.WithTimeout(tb.Context(), assertionTimeout)
	defer cancel()
	for {
		changed := h.out.Changed()
		if cond() {
			return
		}
		select {
		case <-changed:
		case <-ctx.Done():
			tb.Fatalf("programtest: timed out waiting for %s; the last frame was %q", what, h.Frame())
		}
	}
}

// Shows waits until a full repaint contains text.
func (h *Host) Shows(tb testing.TB, text string) {
	tb.Helper()
	h.Until(tb, "the interface to show "+text, func() bool {
		if !h.Repaint() {
			return false
		}
		return strings.Contains(h.Frame(), text)
	})
}

// Hides waits until a full repaint does not contain text.
func (h *Host) Hides(tb testing.TB, text string) {
	tb.Helper()
	h.Until(tb, "the interface to stop showing "+text, func() bool {
		if !h.Repaint() {
			return false
		}
		return !strings.Contains(h.Frame(), text)
	})
}

// Close stops the frame writer. It is idempotent.
func (h *Host) Close() error {
	if h == nil || !h.closed.CompareAndSwap(false, true) {
		return nil
	}
	if h.writer == nil {
		return nil
	}
	h.input.close()
	if err := h.writer.Close(); err != nil {
		return fmt.Errorf("programtest: close frame writer: %w", err)
	}
	return nil
}

// eventSource is an ordered input FIFO with one coalesced wake-up.
//
// A fixed-capacity event channel would make Send block behind work only a program
// that may not have started yet can consume. The queue has no arbitrary burst limit;
// tests choose how many events they retain by how many they send.
type eventSource struct {
	mu     sync.Mutex
	queue  fifo.Queue[input.Event]
	closed bool

	events chan input.Event
	wake   chan struct{}
	done   chan struct{}
	ended  chan struct{}
}

func newEventSource() *eventSource {
	source := &eventSource{
		events: make(chan input.Event),
		wake:   make(chan struct{}, 1),
		done:   make(chan struct{}),
		ended:  make(chan struct{}),
	}
	go source.run()
	return source
}

func (s *eventSource) post(event input.Event) bool {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return false
	}
	s.queue.Push(event)
	s.mu.Unlock()
	select {
	case s.wake <- struct{}{}:
	default:
	}
	return true
}

func (s *eventSource) run() {
	defer close(s.ended)
	defer close(s.events)
	for {
		event, ok := s.take()
		if ok {
			select {
			case s.events <- event:
			case <-s.done:
				return
			}
			continue
		}
		select {
		case <-s.wake:
		case <-s.done:
			return
		}
	}
}

func (s *eventSource) take() (input.Event, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.queue.Pop()
}

func (s *eventSource) close() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	s.queue.Clear()
	close(s.done)
	s.mu.Unlock()
	<-s.ended
}

// sink accumulates terminal output and broadcasts each frame write to every waiter.
type sink struct {
	mu      sync.Mutex
	all     bytes.Buffer
	last    string
	changed chan struct{}
}

func newSink() *sink { return &sink{changed: make(chan struct{})} }

func (s *sink) Write(p []byte) (int, error) {
	s.mu.Lock()
	s.last = string(p)
	n, err := s.all.Write(p)
	changed := s.changed
	s.changed = make(chan struct{})
	s.mu.Unlock()
	close(changed)
	return n, err
}

func (s *sink) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.all.String()
}

func (s *sink) Last() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.last
}

func (s *sink) Changed() <-chan struct{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.changed
}
