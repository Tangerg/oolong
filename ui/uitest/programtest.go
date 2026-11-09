// Package uitest drives a program without opening a terminal.
//
// Host implements [program.Host]'s transport boundary and [program.PointerSource]
// for synthetic pointer intent against completed output. Optional terminal
// capabilities are deliberately absent. A test that needs one can embed *Host in
// its own type and implement that capability, keeping absence and partial support
// testable.
//
// Frames are terminal escape streams, not a simulated screen. Shows and Hides request
// a full frame and inspect its appearance-free text runs: style and hyperlink changes
// are transparent, while cursor movement and erasure remain boundaries. Tests that
// need terminal cell state use package ptytest instead.
package uitest

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Tangerg/oolong/ui/core/ansi"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/program"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/core/term"
)

const assertionTimeout = 5 * time.Second

// Host is an in-memory program host for tests.
//
// Every method is safe to call from the test goroutine while a program session owns the
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

	visibilityMu sync.Mutex
	visibility   frameObservation

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
	if err := grid.ValidateTerminalSize(config.Width, config.Height); err != nil {
		tb.Fatalf("uitest: %v", err)
	}
	out := newSink()
	host := &Host{
		input:  newEventSource(),
		writer: term.NewWriter(term.WriterConfig{Destination: out}),
		out:    out,
		w:      config.Width,
		h:      config.Height,
	}
	tb.Cleanup(func() {
		if err := host.Close(); err != nil {
			tb.Errorf("uitest: close host: %v", err)
		}
	})
	return host
}

func (h *Host) Input() program.EventSource { return h }

func (h *Host) Events() <-chan input.Event {
	if h == nil || h.input == nil {
		return nil
	}
	return h.input.events
}

// Err reports a clean input end. Tests normally stop a program through their
// context or the program's own quit action.
func (h *Host) Err() error { return nil }

func (h *Host) Writer() program.FrameWriter {
	if h == nil || h.writer == nil {
		return nil
	}
	return h.writer.Frames()
}

func (h *Host) Size() (width, height int, err error) {
	if h == nil {
		return 0, 0, nil
	}
	h.sizeMu.Lock()
	defer h.sizeMu.Unlock()
	return h.w, h.h, nil
}

// Send queues a physical terminal report and returns false for invalid input or a closed host.
// It does not wait for the program to receive earlier events.
// Resize events use [Host.Resize] so geometry validation, reported size and input
// admission remain one transition. Pointer intent based on completed output uses
// [Host.Pointer]; a physical timestamp cannot order events within one clock tick.
func (h *Host) Send(event input.Event) bool {
	if h == nil || h.input == nil || input.ByteSize(event) == 0 {
		return false
	}
	if resized, ok := event.(input.Resize); ok {
		return h.Resize(resized.Width, resized.Height)
	}
	return h.input.post(input.Stamp([]input.Event{event}, time.Now())[0])
}

// Pointer captures synthetic intent against the complete output observed at this
// call, before buffering it. It refuses an unbound session or uncertain geometry.
// Leave At zero: explicit physical arrival tests belong to Send. A later frame
// cannot retarget this capture, even when the process clock has not advanced.
func (h *Host) Pointer(mouse input.Mouse) bool {
	if h == nil || h.input == nil || h.writer == nil || !mouse.At.IsZero() {
		return false
	}
	mouse.At = time.Now()
	return h.input.postPointer(mouse, h.writer.WrittenNow)
}

func (h *Host) BindPointerInput(capture func(input.Mouse, uint64) scene.CapturedInput) (<-chan scene.CapturedInput, func(), error) {
	if h == nil || h.input == nil {
		return nil, nil, program.ErrStopped
	}
	return h.input.bindPointer(capture)
}

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

func (h *Host) Press(code input.Code) bool {
	return h.Send(input.Key{Code: code})
}

// Resize changes the reported size and sends the corresponding event.
// Dimensions outside [grid.ValidateTerminalSize] are rejected.
func (h *Host) Resize(width, height int) bool {
	if grid.ValidateTerminalSize(width, height) != nil || h == nil || h.input == nil {
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

// FrameSnapshot records the output through one acknowledged full repaint and the
// size of the painted canvas. Frame is the final write within Frames.
type FrameSnapshot struct {
	Width, Height int
	Frame, Frames string
}

// Repaint requests a full frame after preceding input and waits for its delivery.
// Bound ctx to the program lifetime: a program that stops before taking the request
// cannot acknowledge it. Closing the host also releases the wait.
func (h *Host) Repaint(ctx context.Context) (FrameSnapshot, error) {
	observed, err := h.repaint(ctx)
	return observed.FrameSnapshot, err
}

type frameObservation struct {
	FrameSnapshot
	changed <-chan struct{}
}

func (h *Host) repaint(ctx context.Context) (frameObservation, error) {
	if cause := context.Cause(ctx); cause != nil {
		return frameObservation{}, cause
	}
	if h == nil || h.input == nil {
		return frameObservation{}, program.ErrStopped
	}
	result := make(chan error, 1)
	var snapshot frameObservation
	admitted := h.input.postRepaint(func(width, height int, err error) {
		if err == nil {
			snapshot.Width, snapshot.Height = width, height
			snapshot.Frame, snapshot.Frames, snapshot.changed = h.out.snapshot()
		}
		result <- err
	})
	if !admitted {
		return frameObservation{}, program.ErrStopped
	}
	select {
	case err := <-result:
		if err != nil {
			return frameObservation{}, err
		}
		return snapshot, nil
	case <-ctx.Done():
		return frameObservation{}, context.Cause(ctx)
	case <-h.input.ended:
		return frameObservation{}, program.ErrStopped
	}
}

func (h *Host) Frames() string {
	if h == nil || h.out == nil {
		return ""
	}
	_, frames, _ := h.out.snapshot()
	return frames
}

// Frame returns the most recent frame write.
//
// Ordinary frames are diffs. Use the snapshot returned by Repaint for an assertion
// that needs a full frame, or use Shows and Hides.
func (h *Host) Frame() string {
	if h == nil || h.out == nil {
		return ""
	}
	frame, _, _ := h.out.snapshot()
	return strings.Clone(frame)
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
		tb.Fatal("uitest: cannot wait with an uninitialized host or nil condition")
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
			tb.Fatalf("uitest: timed out waiting for %s; the last frame was %q", what, h.Frame())
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
	for first := true; ; first = false {
		snapshot, err := h.observeFrame(ctx, first)
		if err != nil {
			tb.Fatalf("uitest: waiting for visibility %t of %q: %v; last frame %q", want, text, err, h.Frame())
		}
		visible, err := visibleRuns(snapshot.Frame)
		if err != nil {
			tb.Fatal(err)
		}
		if strings.Contains(visible, text) == want {
			return
		}
		select {
		case <-snapshot.changed:
		case <-h.input.ended:
			tb.Fatalf("uitest: host closed while waiting for visibility %t of %q", want, text)
		case <-ctx.Done():
			tb.Fatalf("uitest: timed out waiting for visibility %t of %q; last frame %q", want, text, h.Frame())
		}
	}
}

// Each caller's first request orders its preceding input. After that barrier,
// simultaneous waiters may share a full frame until a later write invalidates it.
func (h *Host) observeFrame(ctx context.Context, first bool) (frameObservation, error) {
	if h == nil || h.out == nil {
		return frameObservation{}, program.ErrStopped
	}
	h.visibilityMu.Lock()
	defer h.visibilityMu.Unlock()
	if !first && h.visibility.changed == h.out.Changed() {
		return h.visibility, nil
	}
	observation, err := h.repaint(ctx)
	if err == nil {
		// Assertions need only the last frame, not the backing transcript.
		observation.Frame = strings.Clone(observation.Frame)
		observation.Frames = ""
		h.visibility = observation
	}
	return h.visibility, err
}

func (h *Host) Repaints() <-chan func(int, int, error) {
	if h == nil || h.input == nil {
		return nil
	}
	return h.input.repaints
}

// visibleRuns projects a renderer frame into contiguous visible text runs without
// becoming a second terminal model. SGR and OSC hyperlinks change appearance around
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
			// A hyperlink preserves adjacency. An opaque painter (including OSC
			// inline images) is not evidence that the text on its sides is joined.
			if piece.Final != ']' || !strings.HasPrefix(piece.Body, "8;") {
				visible.WriteByte(0)
			}
		case ansi.Control:
			if piece.Final != 'm' || piece.Intermediates != "" || ansi.Parse(piece.Parameters).Marker() != 0 {
				visible.WriteByte(0)
			}
		case ansi.Other:
			if piece.Raw == "\x1b(B" {
				continue
			}
			if strings.HasPrefix(piece.Raw, "\x1b(") {
				return "", fmt.Errorf("unsupported character set %q", piece.Raw)
			}
			visible.WriteByte(0)
		case ansi.Malformed:
			return "", fmt.Errorf("malformed escape stream at %q", piece.Raw)
		}
	}
	return visible.String(), nil
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
				h.closeErr = fmt.Errorf("uitest: close frame writer: %w", err)
			}
		}
	})
	return h.closeErr
}

type sink struct {
	mu      sync.Mutex
	all     strings.Builder
	last    int
	changed chan struct{}
}

func newSink() *sink { return &sink{changed: make(chan struct{})} }

func (s *sink) Write(p []byte) (int, error) {
	s.mu.Lock()
	s.last = s.all.Len()
	n, err := s.all.Write(p)
	changed := s.changed
	s.changed = make(chan struct{})
	s.mu.Unlock()
	close(changed)
	return n, err
}

func (s *sink) snapshot() (frame, frames string, changed <-chan struct{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	frames = s.all.String()
	return frames[s.last:], frames, s.changed
}

func (s *sink) Changed() <-chan struct{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.changed
}
