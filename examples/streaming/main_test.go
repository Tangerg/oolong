package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/graphics"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/program"
	"github.com/Tangerg/oolong/core/term"
)

// host is a terminal-shaped fault boundary: events enter causally through one source
// and encoded frames leave through a synchronized buffer. The deeper program and term
// suites inject partial writes, drain timeouts and teardown faults at this same seam;
// these tests keep the canonical interface focused on its product transitions.
type host struct {
	events chan input.Event
	out    *buffer
	writer *term.Writer
}

func newHost() *host {
	b := &buffer{}
	return &host{events: make(chan input.Event, 64), out: b, writer: term.NewWriter(b)}
}

func (h *host) Input() program.EventSource  { return h }
func (h *host) Events() <-chan input.Event  { return h.events }
func (h *host) Err() error                  { return nil }
func (h *host) Writer() program.FrameWriter { return h.writer }
func (h *host) Size() (int, int, error)     { return 60, 12, nil }
func (h *host) Ground() grid.Ground         { return grid.Ground{} }
func (h *host) Copy(string) bool            { return false }
func (h *host) Paste()                      {}
func (h *host) Wheel() input.Wheel          { return input.Wheel{} }

func (h *host) Graphics() graphics.Protocol   { return graphics.None }
func (h *host) CellSize() (image.Point, bool) { return image.Point{}, false }
func (h *host) Keyboard() (input.KeyboardFlags, bool) {
	return input.KeyboardFlags{}, false
}

func (h *host) Transmit([]byte) (graphics.Image, error) {
	return graphics.Image{}, errors.ErrUnsupported
}
func (h *host) ReportDirectory(string) error { return nil }
func (h *host) SetTitle(string)              {}
func (h *host) Bell()                        {}
func (h *host) Notify(string)                {}

func (h *host) Hand(run func() error) error {
	if run == nil {
		return nil
	}
	return run()
}

func (h *host) typeText(s string) {
	for _, r := range s {
		h.events <- input.Key{Code: input.Character, Rune: r}
	}
}

type buffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (b *buffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.Write(p)
}

func (b *buffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.String()
}

func awaitText(t *testing.T, h *host, want ...string) {
	t.Helper()
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		output := h.out.String()
		found := true
		for _, text := range want {
			found = found && strings.Contains(output, text)
		}
		if found {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("never saw %q on the terminal; what arrived was:\n%s", want, h.out.String())
}

type state struct {
	active, open, dialog bool
	blocks, first        int
	status               string
}

func stateOf(t *testing.T, c *chat) state {
	t.Helper()
	result := make(chan state, 1)
	c.runtime.Dispatcher().Post(func() {
		result <- state{
			active: c.active, open: c.hasOpen, dialog: c.dialog.Open(),
			blocks: c.content.Len(), first: int(c.content.FirstBlock()), status: c.status.Doing,
		}
	})
	select {
	case got := <-result:
		return got
	case <-c.runtime.Dispatcher().Done():
		t.Fatal("the program stopped before its state could be inspected")
	case <-time.After(time.Second):
		t.Fatal("the interface owner did not inspect its state")
	}
	return state{}
}

func awaitState(t *testing.T, c *chat, want func(state) bool) state {
	t.Helper()
	deadline := time.Now().Add(4 * time.Second)
	var got state
	for time.Now().Before(deadline) {
		got = stateOf(t, c)
		if want(got) {
			return got
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("state did not settle; last state was %+v", got)
	return got
}

func runSource(t *testing.T, h *host, source replySource) (*chat, func()) {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	started := make(chan *chat, 1)
	done := make(chan error, 1)
	go func() {
		done <- program.Run(ctx, program.Config{
			Host: h,
			Inline: func(runtime *program.InlineRuntime) program.Component {
				c := newChatWithSource(runtime, source)
				started <- c
				return headless.NewRoot(c)
			},
		})
	}()

	var c *chat
	select {
	case c = <-started:
	case err := <-done:
		t.Fatalf("the program stopped during startup: %v", err)
	case <-time.After(3 * time.Second):
		t.Fatal("the program did not start")
	}
	stop := func() {
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
	return c, stop
}

func approve(t *testing.T, h *host, c *chat, prompt string) {
	t.Helper()
	h.typeText(prompt)
	h.events <- input.Key{Code: input.Enter}
	awaitState(t, c, func(s state) bool { return s.dialog })
	awaitText(t, h, "Approve prompt")
	h.events <- input.Key{Code: input.Enter}
}

func TestThePlaceholderIsUpBeforeAnybodyTypes(t *testing.T) {
	h := newHost()
	_, stop := runSource(t, h, func(context.Context, string, io.Writer) error { return nil })
	defer stop()
	awaitText(t, h, "Ask something")
}

func TestApprovalOwnsTheTransitionIntoBackgroundWork(t *testing.T) {
	h := newHost()
	called := make(chan string, 1)
	c, stop := runSource(t, h, func(_ context.Context, prompt string, dst io.Writer) error {
		called <- prompt
		_, err := io.WriteString(dst, "# Answer\n\napproved\n")
		return err
	})
	defer stop()
	awaitText(t, h, "Ask something")

	h.typeText("what is this")
	h.events <- input.Key{Code: input.Enter}
	awaitText(t, h, "Approve prompt", "Send this prompt?")
	if !stateOf(t, c).dialog {
		t.Fatal("the dialog is visible but its controller is not open")
	}
	select {
	case prompt := <-called:
		t.Fatalf("the source started before approval with %q", prompt)
	default:
	}

	h.events <- input.Key{Code: input.Enter}
	awaitText(t, h, "you", "what is this", "Answer")
	select {
	case prompt := <-called:
		if prompt != "what is this" {
			t.Errorf("source received %q", prompt)
		}
	case <-time.After(time.Second):
		t.Fatal("approval did not start the source")
	}
	awaitState(t, c, func(s state) bool { return !s.active && !s.dialog })
}

func TestStablePrefixesLeaveABoundedInteractiveWindow(t *testing.T) {
	h := newHost()
	chunks := make(chan string)
	c, stop := runSource(t, h, func(ctx context.Context, _ string, dst io.Writer) error {
		for {
			select {
			case <-ctx.Done():
				return context.Cause(ctx)
			case chunk, ok := <-chunks:
				if !ok {
					return nil
				}
				if _, err := io.WriteString(dst, chunk); err != nil {
					return err
				}
			}
		}
	})
	defer stop()
	awaitText(t, h, "Ask something")
	approve(t, h, c, "show the lifecycle")
	awaitText(t, h, "you")

	for i := range 9 {
		chunks <- fmt.Sprintf("section %d\n\n", i)
		want := fmt.Sprintf("section %d", i)
		awaitText(t, h, want)
	}
	close(chunks)
	got := awaitState(t, c, func(s state) bool { return !s.active })
	if got.first == 0 {
		t.Fatal("no stable prefix was transferred to terminal scrollback")
	}
	if got.blocks > retainedFinished {
		t.Fatalf("retained %d finished blocks, bound is %d", got.blocks, retainedFinished)
	}
	if got.open {
		t.Fatal("the open tail survived source completion")
	}
	// The oldest section is absent from the live window but present in encoded terminal
	// output, proving transfer rather than deletion.
	awaitText(t, h, "section 0", "section 8", "complete")
}

func TestSourceFailureKeepsAcceptedOutputAndBecomesDomainState(t *testing.T) {
	h := newHost()
	cause := errors.New("upstream broke")
	c, stop := runSource(t, h, func(_ context.Context, _ string, dst io.Writer) error {
		_, _ = io.WriteString(dst, "accepted before failure\n\nopen tail")
		return cause
	})
	defer stop()
	awaitText(t, h, "Ask something")
	approve(t, h, c, "fail deliberately")
	awaitText(t, h, "accepted before failure", "upstream broke")
	got := awaitState(t, c, func(s state) bool { return !s.active })
	if got.status != "failed: upstream broke" {
		t.Fatalf("status is %q", got.status)
	}
}

func TestCancellationSettlesSeparatelyFromFailure(t *testing.T) {
	h := newHost()
	entered := make(chan struct{})
	c, stop := runSource(t, h, func(ctx context.Context, _ string, _ io.Writer) error {
		close(entered)
		<-ctx.Done()
		return context.Cause(ctx)
	})
	defer stop()
	awaitText(t, h, "Ask something")
	approve(t, h, c, "wait forever")
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("the source did not start")
	}
	h.events <- input.Key{Code: input.Character, Rune: 'x', Mods: input.Ctrl}
	awaitText(t, h, "cancelled")
	got := awaitState(t, c, func(s state) bool { return !s.active })
	if got.status != "cancelled" {
		t.Fatalf("status is %q", got.status)
	}
}

func TestAnEmptyMessageDoesNotOpenApproval(t *testing.T) {
	h := newHost()
	c, stop := runSource(t, h, func(context.Context, string, io.Writer) error {
		t.Fatal("an empty prompt reached the source")
		return nil
	})
	defer stop()
	awaitText(t, h, "Ask something")
	h.events <- input.Key{Code: input.Enter}
	time.Sleep(50 * time.Millisecond)
	if stateOf(t, c).dialog {
		t.Fatal("an empty prompt opened approval")
	}
}

// Commit's consumer contract remains structural across modules.
var _ kit.Printer = (*program.InlineRuntime)(nil)
