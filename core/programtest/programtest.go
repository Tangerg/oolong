// Package programtest drives a program without opening a terminal.
//
// Host implements only [program.Host]'s transport boundary: ordered events in, frames
// out, and an opening cell size. Optional terminal capabilities are deliberately not
// present. A test that needs one can embed *Host in its own type and implement exactly
// that capability, so absence and partial support remain testable rather than being
// hidden by an all-powerful fake.
//
// Frames are terminal escape streams, not a simulated screen. Shows and Hides request
// a full frame and inspect its appearance-free text runs: style and hyperlink changes
// are transparent, while cursor movement and erasure remain boundaries. Tests that
// need terminal cell state use package ptytest instead.
package programtest

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Tangerg/oolong/core/ansi"
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

	closeOnce sync.Once
	closeErr  error
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

// Size returns the current size, initially the dimensions passed to New.
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
// Resize events use [Host.Resize] so geometry validation, reported size and input
// admission remain one transition.
func (h *Host) Send(event input.Event) bool {
	if h == nil || h.input == nil {
		return false
	}
	if resized, ok := event.(input.Resize); ok {
		return h.Resize(resized.Width, resized.Height)
	}
	return h.input.post(event)
}

// Type sends text one character key at a time.
func (h *Host) Type(text string) bool {
	if h == nil || h.input == nil || !h.input.accepting() {
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
	if program.ValidateSize(width, height) != nil || h == nil || h.input == nil {
		return false
	}
	h.sizeMu.Lock()
	defer h.sizeMu.Unlock()
	if !h.input.post(input.Resize{Width: width, Height: height}) {
		return false
	}
	h.w, h.h = width, height
	return true
}

// Repaint asks the program for a full frame at its current size.
func (h *Host) Repaint() bool {
	if h == nil || h.input == nil {
		return false
	}
	return h.input.postRepaint(nil)
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

// Shows waits until one visible text run in a full repaint contains text.
// Appearance changes do not split a run; terminal geometry operations do. This is a
// content assertion rather than a raw escape-stream assertion — use Frame or Frames
// when exact transport bytes are the subject of the test.
func (h *Host) Shows(tb testing.TB, text string) {
	tb.Helper()
	h.assertText(tb, text, true)
}

// Hides waits for a completed full repaint that contains no visible text run matching text.
func (h *Host) Hides(tb testing.TB, text string) {
	tb.Helper()
	h.assertText(tb, text, false)
}

func (h *Host) assertText(tb testing.TB, text string, want bool) {
	tb.Helper()
	ctx, cancel := context.WithTimeout(tb.Context(), assertionTimeout)
	defer cancel()
	for {
		result := make(chan error, 1)
		var frame string
		if h == nil || h.input == nil || !h.input.postRepaint(func(err error) { frame = h.Frame(); result <- err }) {
			tb.Fatal("programtest: repaint source is closed")
		}
		select {
		case err := <-result:
			if err != nil {
				tb.Fatalf("programtest: repaint failed: %v", err)
			}
			visible, err := visibleRuns(frame)
			if err != nil {
				tb.Fatal(err)
			}
			if strings.Contains(visible, text) == want {
				return
			}
		case <-ctx.Done():
			tb.Fatalf("programtest: timed out waiting for visibility %t of %q; last frame %q", want, text, h.Frame())
		}
	}
}

// Repaints is the ordered complete-frame request stream consumed by program.Run.
func (h *Host) Repaints() <-chan func(error) {
	if h == nil || h.input == nil {
		return nil
	}
	return h.input.repaints
}

// visibleRuns projects a renderer frame into contiguous visible text runs without
// becoming a second terminal model. SGR and OSC metadata change appearance around
// adjacent cells and therefore disappear. A sequence that changes cell geometry
// inserts a boundary so text on different rows or across an erasure cannot become a
// false match merely because its escape bytes were removed.
func visibleRuns(frame string) (string, error) {
	var visible strings.Builder
	for frame != "" {
		piece, n, ok := ansi.Next(frame)
		if !ok {
			return "", fmt.Errorf("incomplete escape stream %q", frame)
		}
		frame = frame[n:]
		switch piece.Kind {
		case ansi.Plain:
			visible.WriteString(piece.Raw)
		case ansi.String:
			// OSC hyperlinks and other string commands change metadata, not cell
			// adjacency. Hosts with graphics are outside programtest's deliberately
			// minimal capability set.
		case ansi.Control:
			if !transparentControl(piece.Final) {
				visible.WriteByte(0)
			}
		case ansi.Other:
			if !strings.HasPrefix(piece.Raw, "\x1b(") {
				visible.WriteByte(0)
			}
		case ansi.Malformed:
			return "", fmt.Errorf("malformed escape stream at %q", piece.Raw)
		}
	}
	return visible.String(), nil
}

// transparentControl names renderer controls that cannot change which cells are
// adjacent. Everything not on this short list is a geometry boundary by default.
func transparentControl(final byte) bool {
	switch final {
	case 'c', 'h', 'l', 'm', 'q', 't':
		return true
	default:
		return false
	}
}

// Close ends input and stops the frame writer. Concurrent and repeated calls wait
// for the same settlement and return its teardown and delivery errors.
func (h *Host) Close() error {
	if h == nil {
		return nil
	}
	h.closeOnce.Do(func() {
		if h.input != nil {
			h.input.close()
		}
		if h.writer != nil {
			if err := errors.Join(h.writer.Close(), h.writer.Err()); err != nil {
				h.closeErr = fmt.Errorf("programtest: close frame writer: %w", err)
			}
		}
	})
	return h.closeErr
}

// eventSource is an ordered input FIFO with one coalesced wake-up.
//
// A fixed-capacity event channel would make Send block behind work only a program
// that may not have started yet can consume. The queue has no arbitrary burst limit;
// tests choose how many events they retain by how many they send.
type eventSource struct {
	mu     sync.Mutex
	queue  fifo.Queue[hostEvent]
	closed bool

	events   chan input.Event
	repaints chan func(error)
	wake     chan struct{}
	done     chan struct{}
	ended    chan struct{}
}

func newEventSource() *eventSource {
	source := &eventSource{
		events:   make(chan input.Event),
		repaints: make(chan func(error)),
		wake:     make(chan struct{}, 1),
		done:     make(chan struct{}),
		ended:    make(chan struct{}),
	}
	go source.run()
	return source
}

type hostEvent struct {
	input   input.Event
	repaint bool
	reply   func(error)
}

func (s *eventSource) post(event input.Event) bool { return s.enqueue(hostEvent{input: event}) }

func (s *eventSource) accepting() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return !s.closed
}

func (s *eventSource) postRepaint(reply func(error)) bool {
	return s.enqueue(hostEvent{repaint: true, reply: reply})
}

func (s *eventSource) enqueue(event hostEvent) bool {
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
	defer close(s.repaints)
	for {
		event, ok := s.take()
		if ok {
			if event.repaint {
				select {
				case s.repaints <- event.reply:
				case <-s.done:
					return
				}
				continue
			}
			select {
			case s.events <- event.input:
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

func (s *eventSource) take() (hostEvent, bool) {
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
