package main

import (
	"bytes"
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/program"
	"github.com/Tangerg/oolong/core/term"
)

// The example is driven the way any interface built on this library can be: through
// a [program.Host], with no terminal in sight. That the demonstration is testable at
// all is the claim being made here — a library whose own example can only be checked
// by looking at it is one nobody can build on with confidence.

// host stands in for a terminal: a channel of events in, a buffer of frames out.
type host struct {
	events chan input.Event
	out    *buffer
	writer *term.Writer
}

func newHost() *host {
	b := &buffer{}
	return &host{events: make(chan input.Event, 64), out: b, writer: term.NewWriter(b)}
}

func (h *host) Events() <-chan input.Event { return h.events }
func (h *host) Writer() *term.Writer       { return h.writer }
func (h *host) Size() (int, int, error)    { return 60, 12, nil }
func (h *host) Ground() grid.Ground        { return grid.Ground{} }
func (h *host) Copy(string) bool           { return false }
func (h *host) Paste()                     {}

func (h *host) typeText(s string) {
	for _, r := range s {
		h.events <- input.Key{Code: input.Character, Rune: r}
	}
}

// buffer collects what reached the terminal.
type buffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (f *buffer) Write(p []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.b.Write(p)
}

func (f *buffer) String() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.b.String()
}

// awaitText waits for the terminal to have been sent something containing want.
func awaitText(t *testing.T, h *host, want string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(h.out.String(), want) {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("never saw %q on the terminal; what arrived was:\n%s", want, h.out.String())
}

// run starts the example against a host and returns a function that stops it.
func run(t *testing.T, h *host) func() {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- program.Run(ctx, program.Config{
			Host:   h,
			Inline: func(loop program.InlineLoop) program.Component { return newChat(loop) },
		})
	}()
	return func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("the program ended with %v", err)
			}
		case <-time.After(3 * time.Second):
			t.Error("the program did not stop when its context was cancelled")
		}
	}
}

func TestThePlaceholderIsUpBeforeAnybodyTypes(t *testing.T) {
	// A program draws before it selects, so the interface is there without the user
	// having to press something first.
	h := newHost()
	defer run(t, h)()
	awaitText(t, h, "Ask something")
}

func TestWhatIsTypedAppearsInTheBlock(t *testing.T) {
	h := newHost()
	defer run(t, h)()
	awaitText(t, h, "Ask something")

	h.typeText("hello")
	awaitText(t, h, "hello")
}

func TestSendingPrintsTheMessageAndStartsAnAnswer(t *testing.T) {
	h := newHost()
	defer run(t, h)()
	awaitText(t, h, "Ask something")

	h.typeText("what is this")
	awaitText(t, h, "what is this")
	h.events <- input.Key{Code: input.Enter}

	// The speaker label is only ever drawn by a printed message, so seeing it is
	// seeing the transcript reach the terminal's own scrollback.
	awaitText(t, h, "you")
	// And the answer starts arriving without another keystroke.
	awaitText(t, h, "You said")
}

func TestAnEmptyMessageIsNotSent(t *testing.T) {
	h := newHost()
	defer run(t, h)()
	awaitText(t, h, "Ask something")

	h.events <- input.Key{Code: input.Enter}
	time.Sleep(150 * time.Millisecond)
	if strings.Contains(h.out.String(), "assistant") {
		t.Fatal("pressing enter on an empty composer started an answer")
	}
}

// TestTheLoopCanPrintForATranscript is the seam between the two modules, asserted
// where both are visible: what commits finished output is declared in components and
// satisfied by the loop in core, and neither knows about the other.
var _ kit.Printer = program.InlineLoop(nil)

func (h *host) Wheel() input.Wheel                    { return input.Wheel{} }
func (h *host) Keyboard() (input.KeyboardFlags, bool) { return input.KeyboardFlags{}, false }
func (h *host) ReportDirectory(string) error          { return nil }
