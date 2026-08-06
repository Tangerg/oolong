// Package fake is a terminal that is not one, so the demonstrations beside it can
// be driven and read without opening a real terminal.
//
// It is here rather than in each example's tests because there are seven of them and
// the interesting part of each is the program, not the harness. It is also the answer
// to a question the examples raise by existing: a program built on this library can
// be tested without a terminal, and this is what that looks like from the outside —
// [program.Config.Host] takes anything that can hand over events and take frames.
package fake

import (
	"bytes"
	"errors"
	"image"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Tangerg/oolong/core/graphics"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/term"
)

// Host is somewhere for a program to run that is not a terminal.
type Host struct {
	events chan input.Event
	writer *term.Writer
	out    *sink
	w, h   int

	mu       sync.Mutex
	title    string
	rang     int
	notified []string
	handed   int
}

// New is a host of a given size, with room for a burst of events.
//
// The test is asked for so the host can put itself away: a [term.Writer] has a
// goroutine behind it, and one that is never closed outlives the test that made it.
// Closing it here rather than leaving it to each caller is the difference between a
// rule everybody has to remember and one nobody can break.
func New(t *testing.T, w, h int) *Host {
	t.Helper()
	out := &sink{}
	host := &Host{
		events: make(chan input.Event, 128),
		writer: term.NewWriter(out),
		out:    out,
		w:      w,
		h:      h,
	}
	// After the program: a test's context is cancelled before its cleanups run, so
	// whatever was drawing has been told to stop by the time the writer is closed.
	t.Cleanup(func() { _ = host.writer.Close() })
	return host
}

// The rest is a host answering questions about a terminal there is not. What it
// cannot answer, it answers as the nothing it is — which is exactly what a program
// has to cope with when the terminal it was given turned out to be a pipe.

// Events is the input, closed when the input ends.
func (h *Host) Events() <-chan input.Event { return h.events }

// Writer is where frames go.
func (h *Host) Writer() *term.Writer { return h.writer }

// Size is how big this pretends to be.
func (h *Host) Size() (int, int, error) { return h.w, h.h, nil }

// Ground is what the terminal's own colours are, which here is nothing: a host
// that cannot ask gets the answer a terminal that would not say gives.
func (h *Host) Ground() grid.Ground { return grid.Ground{} }

// Wheel is what a wheel report is worth, which nothing here sends.
func (h *Host) Wheel() input.Wheel { return input.Wheel{} }

// Copy takes the text and says it worked.
func (h *Host) Copy(string) bool { return true }

// Paste has nothing to paste.
func (h *Host) Paste() {}

// ReportDirectory has nobody to tell.
func (h *Host) ReportDirectory(string) error { return nil }

// Keyboard is which keyboard enhancements are on, which here is none of them.
func (h *Host) Keyboard() (input.KeyboardFlags, bool) { return input.KeyboardFlags{}, false }

// Graphics is how this will take a picture, which is not at all.
func (h *Host) Graphics() graphics.Protocol { return graphics.None }

// CellSize is how many pixels a cell is, which nothing here knows.
func (h *Host) CellSize() (image.Point, bool) { return image.Point{}, false }

// Transmit has nowhere to send a picture.
func (h *Host) Transmit([]byte) (graphics.Image, error) {
	return graphics.Image{}, errors.ErrUnsupported
}

// Hand runs it. There is no terminal here to give away, and the half worth testing
// is the program's: that it stops drawing while something else has the screen.
func (h *Host) Hand(run func() error) error {
	h.mu.Lock()
	h.handed++
	h.mu.Unlock()
	if run == nil {
		return nil
	}
	return run()
}

// SetTitle records what the window would have been called.
func (h *Host) SetTitle(s string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.title = s
}

// Bell records that attention was asked for.
func (h *Host) Bell() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.rang++
}

// Notify records what would have been sent to the desktop.
func (h *Host) Notify(text string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.notified = append(h.notified, text)
}

// Said is what the program told the terminal beside its frames.
func (h *Host) Said() (title string, rang int, notified []string, handed int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.title, h.rang, append([]string(nil), h.notified...), h.handed
}

// Send puts an event in front of the program.
func (h *Host) Send(ev input.Event) { h.events <- ev }

// Type sends what somebody typed, one keystroke per character.
func (h *Host) Type(s string) {
	for _, r := range s {
		h.Send(input.Key{Code: input.Character, Rune: r})
	}
}

// Press sends a key that is not a character.
func (h *Host) Press(code input.Code) { h.Send(input.Key{Code: code}) }

// Frames is everything that has reached the terminal so far, escape sequences and
// all.
func (h *Host) Frames() string { return h.out.String() }

// Frame is the last one written, which is the only one worth searching: a frame is
// one write, and an ordinary one holds the few cells that changed rather than what is
// on the screen. [Host.Repaint] is what makes the next one hold everything.
func (h *Host) Frame() string { return h.out.Last() }

// Repaint asks the program to draw everything again.
//
// A resize is how: whatever else it means, it means the terminal's contents are no
// longer known. It is what a test does before reading a frame, because the interface
// is only ever written to the terminal as the difference from the last one — which is
// the whole point of the renderer and the one thing that makes a frame hard to read
// from outside.
func (h *Host) Repaint() { h.Send(input.Resize{Width: h.w, Height: h.h}) }

// Until waits for something to become true, failing the test if it never does.
func (h *Host) Until(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s; the last frame was %q", what, h.Frame())
}

// Shows waits for text to appear on the screen, which is what most of these tests
// are asking about.
func (h *Host) Shows(t *testing.T, text string) {
	t.Helper()
	h.Until(t, "the interface to show "+text, func() bool {
		h.Repaint()
		return strings.Contains(h.Frame(), text)
	})
}

// Hides waits for text to stop being on the screen.
func (h *Host) Hides(t *testing.T, text string) {
	t.Helper()
	h.Until(t, "the interface to stop showing "+text, func() bool {
		h.Repaint()
		return !strings.Contains(h.Frame(), text)
	})
}

// sink collects what reached the terminal, and keeps the last frame apart from the
// rest: a frame is one write.
type sink struct {
	mu   sync.Mutex
	b    bytes.Buffer
	last string
}

func (s *sink) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.last = string(p)
	return s.b.Write(p)
}

func (s *sink) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

func (s *sink) Last() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.last
}
