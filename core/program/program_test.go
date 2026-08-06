package program_test

import (
	"bytes"
	"context"
	"errors"
	"image"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/Tangerg/oolong/core/program"

	"github.com/Tangerg/oolong/core/graphics"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/term"
)

// Everything here is asserted against the bytes that reached the terminal, or against
// state the test itself owns behind a lock.
//
// Nothing reads what the component holds without going through the loop. That state
// belongs to the program's goroutine, and a test that reached into it would be a data
// race dressed up as a test — the very thing this package exists to make unnecessary.

// host stands in for a terminal: a channel of events in, a buffer of frames out.
type host struct {
	events    chan input.Event
	frames    *frames
	writer    *term.Writer
	w, h      int
	ground    grid.Ground
	wheel     input.Wheel
	keys      input.KeyboardFlags
	saidKeys  bool
	directory string

	// clip stands in for a system clipboard, so a test can assert on what was put
	// there instead of on the bytes that would have asked a terminal to do it.
	clipMu   sync.Mutex
	copied   []string
	refuse   bool
	pasteFor string
	asked    int
	handed   int
	title    string
	rang     int
	notified []string
}

// newHost is somewhere for a program to run that is not a terminal.
//
// The test is asked for so the host can put itself away: a [term.Writer] has a
// goroutine behind it, and one that is never closed outlives the test that made it.
// A test's context is cancelled before its cleanups run, so whatever was drawing has
// been told to stop by the time the writer is closed.
func newHost(t *testing.T) *host {
	t.Helper()
	f := &frames{}
	h := &host{
		events: make(chan input.Event, 64),
		frames: f,
		writer: term.NewWriter(f),
		w:      40,
		h:      10,
	}
	t.Cleanup(func() { _ = h.writer.Close() })
	return h
}

func (h *host) Events() <-chan input.Event { return h.events }
func (h *host) Writer() *term.Writer       { return h.writer }
func (h *host) Size() (int, int, error)    { return h.w, h.h, nil }

// Ground says what a real terminal would only say if it were asked. A host that
// can answer this is a host that can put an interface's look under test both ways
// round, which no amount of driving keystrokes could.
func (h *host) Ground() grid.Ground { return h.ground }

func (h *host) Copy(text string) bool {
	h.clipMu.Lock()
	defer h.clipMu.Unlock()
	if h.refuse {
		return false
	}
	h.copied = append(h.copied, text)
	return true
}

// Paste answers the way a host that is not a terminal would: by delivering the
// text itself. Nothing above cares which it was.
func (h *host) Paste() {
	h.clipMu.Lock()
	text, deliver := h.pasteFor, h.pasteFor != ""
	h.asked++
	h.clipMu.Unlock()
	if deliver {
		h.events <- input.Paste{Text: text}
	}
}

// Hand runs it. A host with no terminal has nothing to give away, and the half
// worth testing is the program's: that the block is left behind first, that nothing
// is drawn while it is gone, and that the interface is drawn again afterwards.
func (h *host) Hand(run func() error) error {
	h.clipMu.Lock()
	h.handed++
	h.clipMu.Unlock()
	if run == nil {
		return nil
	}
	return run()
}

// SetTitle, Bell and Notify record rather than send. What a host with no terminal
// does with them is nothing; what a test wants to know is that they were said.
func (h *host) SetTitle(s string) {
	h.clipMu.Lock()
	defer h.clipMu.Unlock()
	h.title = s
}

func (h *host) Bell() {
	h.clipMu.Lock()
	defer h.clipMu.Unlock()
	h.rang++
}

func (h *host) Notify(text string) {
	h.clipMu.Lock()
	defer h.clipMu.Unlock()
	h.notified = append(h.notified, text)
}

func (h *host) said() (title string, rang int, notified []string) {
	h.clipMu.Lock()
	defer h.clipMu.Unlock()
	return h.title, h.rang, slices.Clone(h.notified)
}

// Graphics, CellSize and Transmit answer the way a host with no terminal has to:
// there is nowhere to put a picture, and saying so is what lets an interface draw
// something else instead.
func (h *host) Graphics() graphics.Protocol { return graphics.None }

func (h *host) CellSize() (image.Point, bool) { return image.Point{}, false }

func (h *host) Transmit([]byte) (graphics.Image, error) {
	return graphics.Image{}, errors.ErrUnsupported
}

func (h *host) timesHanded() int {
	h.clipMu.Lock()
	defer h.clipMu.Unlock()
	return h.handed
}

func (h *host) copies() []string {
	h.clipMu.Lock()
	defer h.clipMu.Unlock()
	return slices.Clone(h.copied)
}

func (h *host) timesAsked() int {
	h.clipMu.Lock()
	defer h.clipMu.Unlock()
	return h.asked
}

func (h *host) send(ev input.Event) { h.events <- ev }
func (h *host) rune(r rune)         { h.send(input.Key{Code: input.Character, Rune: r}) }

// frames collects what reached the terminal.
type frames struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (f *frames) Write(p []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.b.Write(p)
}

func (f *frames) String() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.b.String()
}

func (f *frames) size() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.b.Len()
}

// component is a test component: it draws a line of text and counts what it was given.
//
// Its counters are atomic because a test reads them; the text it draws is only ever
// touched on the program's goroutine, which is the rule this package promises.
type component struct {
	loop program.Loop

	text    string
	handled atomic.Int64
	drawn   atomic.Int64
	// consume decides whether Handle claims what it is given.
	consume bool
}

func (c *component) Draw(v grid.View) {
	c.drawn.Add(1)
	v.Text(0, 0, c.text, grid.Style{})
}

func (c *component) Handle(ev input.Event) bool {
	c.handled.Add(1)
	if key, ok := ev.(input.Key); ok && key.Code == input.Character {
		c.text += string(key.Rune)
	}
	return c.consume
}

// running is a program under test.
type running struct {
	host *host
	root *component
	done chan error
	t    *testing.T
}

// start runs a program over a fake host.
func start(t *testing.T, prepare func(*component)) *running {
	t.Helper()
	h := newHost(t)
	root := &component{text: "ready", consume: true}
	done := make(chan error, 1)
	go func() {
		done <- program.Run(t.Context(), program.Config{
			Host: h,
			Root: func(loop program.Loop) program.Component {
				root.loop = loop
				if prepare != nil {
					prepare(root)
				}
				return root
			},
		})
	}()
	return &running{host: h, root: root, done: done, t: t}
}

// until waits for something to become true, failing if the program ends first.
func (r *running) until(what string, cond func() bool) {
	r.t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		select {
		case err := <-r.done:
			r.t.Fatalf("the program ended waiting for %s: %v", what, err)
		case <-time.After(2 * time.Millisecond):
		}
	}
	r.t.Fatalf("timed out waiting for %s", what)
}

func (r *running) wait() error {
	r.t.Helper()
	select {
	case err := <-r.done:
		return err
	case <-time.After(10 * time.Second):
		r.t.Fatal("the program never returned")
		return nil
	}
}

func TestTheInterfaceIsDrawnBeforeAnythingHappens(t *testing.T) {
	// A program draws before it selects. Waiting for the user to press something would
	// leave a blank terminal in front of them.
	r := start(t, nil)
	r.until("the opening frame", func() bool {
		return strings.Contains(r.host.frames.String(), "ready")
	})
	r.root.loop.Quit()
	if err := r.wait(); err != nil {
		t.Fatalf("program: %v", err)
	}
}

func TestInputReachesTheComponent(t *testing.T) {
	r := start(t, nil)
	r.until("the opening frame", func() bool { return r.host.frames.size() > 0 })
	r.host.rune('!')
	r.until("the keystroke to be handled", func() bool { return r.root.handled.Load() == 1 })
	r.root.loop.Quit()
	if err := r.wait(); err != nil {
		t.Fatalf("program: %v", err)
	}
}

func TestAnEventTheComponentDeclinesIsDropped(t *testing.T) {
	// A component is the root of its own tree. There is nobody above it to pass an
	// unclaimed event on to, and pretending otherwise would mean inventing a policy for
	// events nobody wanted.
	r := start(t, func(c *component) { c.consume = false })
	r.until("the opening frame", func() bool { return r.host.frames.size() > 0 })
	r.host.rune('x')
	r.until("the keystroke to be offered", func() bool { return r.root.handled.Load() == 1 })
	r.root.loop.Quit()
	if err := r.wait(); err != nil {
		t.Fatalf("program: %v", err)
	}
}

func TestPostRunsOnTheProgramsGoroutineAndThenDraws(t *testing.T) {
	// The whole of the concurrency model: work that happened elsewhere is applied where
	// the state lives, and what it changed is shown.
	r := start(t, nil)
	r.until("the opening frame", func() bool { return r.host.frames.size() > 0 })

	done := make(chan struct{})
	r.root.loop.Post(func() {
		r.root.text = "posted"
		close(done)
	})
	<-done
	r.until("the posted change to be drawn", func() bool {
		return strings.Contains(r.host.frames.String(), "posted")
	})
	r.root.loop.Quit()
	if err := r.wait(); err != nil {
		t.Fatalf("program: %v", err)
	}
}

func TestRefreshAsksForAFrame(t *testing.T) {
	r := start(t, nil)
	r.until("the opening frame", func() bool { return r.host.frames.size() > 0 })
	settled := r.root.drawn.Load()
	r.root.loop.Refresh()
	r.until("another frame", func() bool { return r.root.drawn.Load() > settled })
	r.root.loop.Quit()
	if err := r.wait(); err != nil {
		t.Fatalf("program: %v", err)
	}
}

// The tests about a clock are the ones that have to prove a negative: that nothing
// ticked after it was stopped, that an idle program wrote nothing more. Waiting a
// while and looking again only ever proves that nothing happened *yet*, and it costs
// the wait every run. Inside a bubble the clock is this test's own — it moves only
// when every goroutine in here has nothing left to do — so "and then nothing happened
// for sixty milliseconds" is a statement about the program rather than about the
// machine it ran on, and it is instant. It also holds the program to a second claim
// nobody had written down: a bubble does not end until the goroutines inside it do.

func TestEveryTicksUntilItIsStopped(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		r := start(t, nil)
		r.until("the opening frame", func() bool { return r.host.frames.size() > 0 })

		var ticks atomic.Int64
		stop := r.root.loop.Every(5*time.Millisecond, func() { ticks.Add(1) })
		r.until("the clock to tick", func() bool { return ticks.Load() >= 3 })

		stop()
		time.Sleep(30 * time.Millisecond)
		settled := ticks.Load()
		time.Sleep(60 * time.Millisecond)
		if grown := ticks.Load() - settled; grown != 0 {
			t.Fatalf("the clock ticked %d more times after being stopped", grown)
		}
		// Stopping twice is not an error: an owner that stops a clock on more than one
		// path should not have to remember which one ran.
		stop()

		r.root.loop.Quit()
		if err := r.wait(); err != nil {
			t.Fatalf("program: %v", err)
		}
	})
}

func TestEveryStopsWhenTheProgramDoes(t *testing.T) {
	// A clock nobody stopped must not outlive the program it was drawing for.
	synctest.Test(t, func(t *testing.T) {
		r := start(t, nil)
		r.until("the opening frame", func() bool { return r.host.frames.size() > 0 })

		var ticks atomic.Int64
		r.root.loop.Every(5*time.Millisecond, func() { ticks.Add(1) })
		r.until("the clock to tick", func() bool { return ticks.Load() >= 2 })

		r.root.loop.Quit()
		if err := r.wait(); err != nil {
			t.Fatalf("program: %v", err)
		}
		time.Sleep(30 * time.Millisecond)
		settled := ticks.Load()
		time.Sleep(60 * time.Millisecond)
		if grown := ticks.Load() - settled; grown != 0 {
			t.Fatalf("the clock ticked %d more times after the program ended", grown)
		}
	})
}

func TestAnIntervalOfNothingIsNotAClock(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		r := start(t, nil)
		r.until("the opening frame", func() bool { return r.host.frames.size() > 0 })

		var ticks atomic.Int64
		stop := r.root.loop.Every(0, func() { ticks.Add(1) })
		stop()
		if r.root.loop.Every(time.Millisecond, nil) == nil {
			t.Fatal("a clock with nothing to call returned no way to stop it")
		}
		time.Sleep(20 * time.Millisecond)
		if ticks.Load() != 0 {
			t.Fatal("a clock with no interval ticked anyway")
		}
		r.root.loop.Quit()
		if err := r.wait(); err != nil {
			t.Fatalf("program: %v", err)
		}
	})
}

func TestPostAfterTheProgramHasStoppedIsDropped(t *testing.T) {
	// Blocking a caller for ever would be worse than losing work there is nothing left
	// to show.
	r := start(t, nil)
	r.until("the opening frame", func() bool { return r.host.frames.size() > 0 })
	loop := r.root.loop
	loop.Quit()
	if err := r.wait(); err != nil {
		t.Fatalf("program: %v", err)
	}

	settled := make(chan struct{})
	go func() {
		defer close(settled)
		for range 100 {
			loop.Post(func() {})
		}
		loop.Refresh()
	}()
	select {
	case <-settled:
	case <-time.After(5 * time.Second):
		t.Fatal("posting to a program that has stopped blocked the caller")
	}
}

func TestTheInputEndingEndsTheProgram(t *testing.T) {
	r := start(t, nil)
	r.until("the opening frame", func() bool { return r.host.frames.size() > 0 })
	close(r.host.events)
	if err := r.wait(); err != nil {
		t.Fatalf("program: %v", err)
	}
}

func TestACancelledContextEndsTheProgramWithoutAnError(t *testing.T) {
	// Being asked to stop is not a failure.
	h := newHost(t)
	root := &component{text: "ready", consume: true}
	done := make(chan error, 1)
	ctx, cancel := context.WithCancel(t.Context())
	go func() {
		done <- program.Run(ctx, program.Config{Host: h, Root: func(program.Loop) program.Component { return root }})
	}()
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("program: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the program ignored the cancellation")
	}
}

func TestAResizeChangesTheGeometryAndRepaints(t *testing.T) {
	r := start(t, nil)
	r.until("the opening frame", func() bool { return r.host.frames.size() > 0 })
	r.host.send(input.Resize{Width: 20, Height: 4})
	// A full repaint at the new size positions the cursor on a row only the new geometry
	// has.
	r.until("a frame at the new size", func() bool {
		return strings.Contains(r.host.frames.String(), "\x1b[4;")
	})
	// It is the program's own event: the component is never offered it, because it has
	// nothing to decide about it.
	if got := r.root.handled.Load(); got != 0 {
		t.Fatalf("the component was offered %d events, want none", got)
	}
	r.root.loop.Quit()
	if err := r.wait(); err != nil {
		t.Fatalf("program: %v", err)
	}
}

func TestRegainingFocusRepaints(t *testing.T) {
	// Another program may have written to the terminal while this one was not in front,
	// so what it is showing can no longer be assumed.
	r := start(t, nil)
	r.until("the opening frame", func() bool { return r.host.frames.size() > 0 })
	settled := r.host.frames.size()
	r.host.send(input.FocusIn{})
	r.until("a repaint", func() bool { return r.host.frames.size() > settled })
	if got := r.root.handled.Load(); got != 0 {
		t.Fatalf("the component was offered %d events, want none", got)
	}
	r.root.loop.Quit()
	if err := r.wait(); err != nil {
		t.Fatalf("program: %v", err)
	}
}

func TestAnIdleProgramStopsWriting(t *testing.T) {
	// The reason the presenter exists: a terminal written to on every pass is a terminal
	// whose cursor never blinks and whose remote session never settles.
	//
	// Four hundred milliseconds of a bubble's clock, which pass the moment there is
	// nothing left for the program to do — so this asks whether an idle program has
	// anything to do at all, rather than whether it managed to stay quiet for a while.
	synctest.Test(t, func(t *testing.T) {
		r := start(t, nil)
		r.until("the opening frame", func() bool { return r.host.frames.size() > 0 })
		time.Sleep(150 * time.Millisecond)
		settled := r.host.frames.size()
		time.Sleep(400 * time.Millisecond)
		if grown := r.host.frames.size() - settled; grown != 0 {
			t.Fatalf("an idle program wrote %d more bytes", grown)
		}
		r.root.loop.Quit()
		if err := r.wait(); err != nil {
			t.Fatalf("program: %v", err)
		}
	})
}

func TestABurstOfUpdatesIsCoalescedIntoFewerFrames(t *testing.T) {
	// A stream of updates arriving faster than a terminal can be redrawn must not become
	// a frame each, or the interface spends its time drawing instead of keeping up.
	synctest.Test(t, func(t *testing.T) {
		r := start(t, nil)
		r.until("the opening frame", func() bool { return r.host.frames.size() > 0 })
		before := r.root.drawn.Load()

		const updates = 200
		settled := make(chan struct{})
		for i := range updates {
			r.root.loop.Post(func() {
				r.root.text = strings.Repeat(".", i%20)
				if i == updates-1 {
					close(settled)
				}
			})
		}
		<-settled
		// Long enough for every frame the throttle held back to come due, which in a
		// bubble is however long the program still has something waiting to happen.
		time.Sleep(100 * time.Millisecond)
		if frames := r.root.drawn.Load() - before; frames >= updates {
			t.Fatalf("%d updates produced %d frames, want them coalesced", updates, frames)
		}
		r.root.loop.Quit()
		if err := r.wait(); err != nil {
			t.Fatalf("program: %v", err)
		}
	})
}

func TestTheLastUpdateOfABurstIsStillDrawn(t *testing.T) {
	// Coalescing must not lose the tail: a frame turned away for arriving too soon has
	// to come due, or the final state of a stream is never shown.
	r := start(t, nil)
	r.until("the opening frame", func() bool { return r.host.frames.size() > 0 })
	for range 20 {
		r.root.loop.Post(func() { r.root.text = "middle" })
	}
	r.root.loop.Post(func() { r.root.text = "final" })
	r.until("the last update to be drawn", func() bool {
		return strings.Contains(r.host.frames.String(), "final")
	})
	r.root.loop.Quit()
	if err := r.wait(); err != nil {
		t.Fatalf("program: %v", err)
	}
}

func TestAFailedTerminalEndsTheProgramWithItsError(t *testing.T) {
	// An interface that cannot reach its terminal has nothing left to do, and a loop
	// that kept going would spin on the same error for ever.
	h := newHost(t)
	h.writer = term.NewWriter(brokenTerminal{})
	done := make(chan error, 1)
	go func() {
		done <- program.Run(t.Context(), program.Config{
			Host: h,
			Root: func(program.Loop) program.Component { return &component{text: "ready"} },
		})
	}()
	select {
	case err := <-done:
		if !errors.Is(err, errBrokenTerminal) {
			t.Fatalf("program returned %v, want the write failure", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the program kept going after the terminal failed")
	}
}

func TestAProgramWithNoComponentIsRefused(t *testing.T) {
	if err := program.Run(t.Context(), program.Config{Host: newHost(t)}); err == nil {
		t.Fatal("a program with nothing to draw was accepted")
	}
}

func TestTheComponentIsGivenItsLoopBeforeItIsDrawn(t *testing.T) {
	// So that a component can hand the loop to whatever fetches on its behalf from the
	// moment it exists, rather than having to wait for a first frame.
	h := newHost(t)
	done := make(chan error, 1)
	// The loop comes back over a channel rather than through a variable the test
	// watches: the claim is that the component was handed one when it was built, and
	// a channel says exactly that, at the moment it happened.
	built := make(chan program.Loop, 1)
	go func() {
		done <- program.Run(t.Context(), program.Config{
			Host: h,
			Root: func(loop program.Loop) program.Component {
				built <- loop
				return &component{text: "ready"}
			},
		})
	}()
	var quit program.Loop
	select {
	case quit = <-built:
	case err := <-done:
		t.Fatalf("the program ended before it built anything: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("the component was never built")
	}
	if quit == nil {
		t.Fatal("the component was built without a loop")
	}
	quit.Quit()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("program: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the program never returned")
	}
}

// brokenTerminal fails every write.
type brokenTerminal struct{}

var errBrokenTerminal = errors.New("the terminal went away")

func (brokenTerminal) Write([]byte) (int, error) { return 0, errBrokenTerminal }

// The inline mode's own tests. What they are about is placement: an interface that
// shares the terminal has to reach its own rows without ever naming one, and has to
// leave the terminal usable when it stops.

// printer is a component that draws a line and can send finished output above itself.
type printer struct {
	loop program.InlineLoop

	text  string
	drawn atomic.Int64
	// hand is what to give the terminal to on the next event, if anything.
	hand func() error
	// handErr is what handing it over came to.
	handErr atomic.Pointer[error]
}

func (p *printer) Draw(v grid.View) {
	p.drawn.Add(1)
	v.Text(0, 0, p.text, grid.Style{})
}

func (p *printer) Handle(input.Event) bool {
	if p.hand != nil {
		run := p.hand
		p.hand = nil
		err := p.loop.Hand(run)
		p.handErr.Store(&err)
	}
	return true
}

// startInline runs an inline program over a fake host.
func startInline(t *testing.T) (*host, *printer, chan error) {
	t.Helper()
	h := newHost(t)
	root := &printer{text: "prompt"}
	done := make(chan error, 1)
	go func() {
		done <- program.Run(t.Context(), program.Config{
			Host: h,
			Inline: func(loop program.InlineLoop) program.Component {
				root.loop = loop
				return root
			},
		})
	}()
	waitFor(t, done, h, "the inline interface to draw", func() bool {
		return strings.Contains(h.frames.String(), "prompt")
	})
	return h, root, done
}

func waitFor(t *testing.T, done chan error, h *host, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		select {
		case err := <-done:
			t.Fatalf("the program ended waiting for %s: %v (frames: %q)", what, err, h.frames.String())
		case <-time.After(2 * time.Millisecond):
		}
	}
	t.Fatalf("timed out waiting for %s (frames: %q)", what, h.frames.String())
}

func TestExactlyOneRootSaysWhereTheInterfaceGoes(t *testing.T) {
	// Which of the two is set decides the rendering model, so neither and both are
	// equally unanswerable.
	both := program.Config{
		Root:   func(program.Loop) program.Component { return &component{} },
		Inline: func(program.InlineLoop) program.Component { return &printer{} },
	}
	for what, cfg := range map[string]program.Config{"neither": {}, "both": both} {
		if err := program.Run(t.Context(), cfg); err == nil {
			t.Errorf("%s root was accepted", what)
		}
	}
}

func TestAnInlineInterfaceCannotTakeTheAlternateScreen(t *testing.T) {
	// A caller who asked for both believes something false: an interface on a screen
	// of its own has no session output to sit among, and nowhere to print.
	err := program.Run(t.Context(), program.Config{
		Inline:   func(program.InlineLoop) program.Component { return &printer{} },
		Terminal: term.Options{AltScreen: true},
		Host:     newHost(t),
	})
	if err == nil {
		t.Fatal("asking for an inline interface on the alternate screen was accepted")
	}
}

func TestAnInlineInterfaceNeverNamesARowOfTheTerminal(t *testing.T) {
	// The program's rows are wherever the session's output left them, and nothing here
	// is allowed to assume otherwise.
	h, root, done := startInline(t)
	root.loop.Post(func() { root.text = "typing" })
	waitFor(t, done, h, "the next frame", func() bool {
		return strings.Contains(h.frames.String(), "typing")
	})
	root.loop.Quit()
	if err := <-done; err != nil {
		t.Fatalf("program: %v", err)
	}
	if got := h.frames.String(); strings.Contains(got, "\x1b[1;1H") || strings.Contains(got, ";1H") {
		t.Fatalf("an inline frame addressed a row of the terminal: %q", got)
	}
}

func TestPrintedOutputReachesTheTerminalAboveTheInterface(t *testing.T) {
	h, root, done := startInline(t)
	root.loop.PrintRows(1, func(v grid.View) { v.Text(0, 0, "finished", grid.Style{}) })
	waitFor(t, done, h, "the printed row", func() bool {
		return strings.Contains(h.frames.String(), "finished")
	})
	root.loop.Quit()
	if err := <-done; err != nil {
		t.Fatalf("program: %v", err)
	}

	got := h.frames.String()
	printed := strings.Index(got, "finished")
	// The interface is drawn again below what was printed, which is what pushes the
	// printed row up and into the terminal's scrollback.
	if after := strings.Index(got[printed:], "prompt"); after < 0 {
		t.Fatalf("the interface was not redrawn below the printed row: %q", got)
	}
}

func TestPrintedOutputIsNotLostToTheExit(t *testing.T) {
	// Printing and then quitting in the same breath is what a one-shot run does: say
	// the answer, then stop. Pacing must not be able to swallow the answer.
	h, root, done := startInline(t)
	root.loop.PrintRows(1, func(v grid.View) { v.Text(0, 0, "the answer", grid.Style{}) })
	root.loop.Quit()
	if err := <-done; err != nil {
		t.Fatalf("program: %v", err)
	}
	if got := h.frames.String(); !strings.Contains(got, "the answer") {
		t.Fatalf("the printed row never reached the terminal: %q", got)
	}
}

func TestTheLastStateOfAnInlineInterfaceIsWhatStays(t *testing.T) {
	// It is not taken down on the way out — it is the last thing the session said, and
	// the cursor has to end up below it so the next prompt does not land on top.
	h, root, done := startInline(t)
	root.loop.Post(func() { root.text = "goodbye" })
	root.loop.Quit()
	if err := <-done; err != nil {
		t.Fatalf("program: %v", err)
	}
	got := h.frames.String()
	if !strings.Contains(got, "goodbye") {
		t.Fatalf("the last state was never drawn: %q", got)
	}
	if !strings.HasSuffix(got, "\r\n\x1b[0m\x1b[?25h") {
		t.Fatalf("the terminal was not handed back below the interface: %q", got)
	}
}

// measurer is a Printable built from two functions.
type measurer struct {
	measure func(width int) int
	draw    func(v grid.View)
}

func (m measurer) Measure(width int) int { return m.measure(width) }
func (m measurer) Draw(v grid.View)      { m.draw(v) }

func TestPrintingMeasuresAgainstTheBlocksOwnWidth(t *testing.T) {
	// The width is the program's to know. A caller that had to remember how wide the
	// last frame was would be keeping a copy of state the loop already has — and
	// measuring a mutable widget from their own goroutine while they were at it.
	h, root, done := startInline(t)

	measured := make(chan int, 1)
	root.loop.Print(measurer{
		measure: func(width int) int { measured <- width; return 1 },
		draw:    func(v grid.View) { v.Text(0, 0, "printed", grid.Style{}) },
	})
	waitFor(t, done, h, "the printed row", func() bool {
		return strings.Contains(h.frames.String(), "printed")
	})
	root.loop.Quit()
	if err := <-done; err != nil {
		t.Fatalf("program: %v", err)
	}
	if got := <-measured; got != h.w {
		t.Fatalf("measured against width %d, want the block's own %d", got, h.w)
	}
}

func TestAppendedOutputIsFedRowsUntilItSaysItIsDone(t *testing.T) {
	// A caller laying text out has no way to know how much of the row is left until
	// the loop tells it, and no way to ask from its own goroutine without racing the
	// interface. So the loop asks, with what is left, and keeps asking with whole rows
	// until the caller says there is no more.
	h, root, done := startInline(t)

	offered := make(chan int, 4)
	rest := "one two three four five six seven eight nine"
	root.loop.Append(func(v grid.View) bool {
		w, _ := v.Size()
		offered <- w
		take := min(len(rest), w)
		v.Text(0, 0, rest[:take], grid.Style{})
		rest = rest[take:]
		return rest != ""
	})
	waitFor(t, done, h, "the appended output", func() bool {
		return strings.Contains(h.frames.String(), "nine")
	})
	root.loop.Quit()
	if err := <-done; err != nil {
		t.Fatalf("program: %v", err)
	}
	close(offered)
	first := <-offered
	if first != h.w {
		t.Fatalf("the first piece was offered %d columns, want the whole row %d", first, h.w)
	}
	for w := range offered {
		if w != h.w {
			t.Fatalf("a later piece was offered %d columns, want a whole row", w)
		}
	}
}

func TestAppendingNothingForeverStillEnds(t *testing.T) {
	// A caller that says there is more and draws nothing into a whole row is asking
	// for room that would not help. The loop stops rather than spinning on it, because
	// it is the goroutine the interface is drawn on.
	h, root, done := startInline(t)
	rounds := make(chan struct{}, 8)
	root.loop.Append(func(grid.View) bool {
		rounds <- struct{}{}
		return true
	})
	root.loop.Post(func() {}) // work queued behind it, which only runs if the loop got free
	waitFor(t, done, h, "the append to give up", func() bool { return len(rounds) > 0 })
	root.loop.Quit()
	if err := <-done; err != nil {
		t.Fatalf("program: %v", err)
	}
	_ = h
}

func TestPrintingNothingIsIgnored(t *testing.T) {
	h, root, done := startInline(t)
	root.loop.Print(nil) // must not reach the loop's goroutine and panic there
	root.loop.Quit()
	if err := <-done; err != nil {
		t.Fatalf("program: %v", err)
	}
	_ = h
}

// startOn runs a program over a host the caller prepared, so a test can say what
// the terminal would have answered. The program is stopped when the test ends.
func startOn(t *testing.T, h *host, root func(program.Loop) program.Component) *running {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- program.Run(ctx, program.Config{Host: h, Root: root}) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("the program never returned after its context was cancelled")
		}
	})
	return &running{host: h, done: done, t: t}
}

func TestAComponentLearnsWhatTheTerminalDrawsOn(t *testing.T) {
	// The one fact a look cannot be built without and cannot work out for itself.
	// A component reads it from the loop it already holds.
	h := newHost(t)
	h.ground = grid.Ground{BG: grid.RGBColor(0xfd, 0xf6, 0xe3)}

	learned := make(chan grid.Ground, 1)
	r := startOn(t, h, func(l program.Loop) program.Component {
		learned <- l.Ground()
		return &component{text: "ready", consume: true, loop: l}
	})
	r.until("the opening frame", func() bool { return h.frames.size() > 0 })

	got := <-learned
	if got.BG.Default() {
		t.Fatal("the component was told the background was unknown")
	}
	if want := (grid.RGB{R: 0xfd, G: 0xf6, B: 0xe3}); got.BG.RGB() != want {
		t.Errorf("background = %+v, want %+v", got.BG.RGB(), want)
	}
	if got.BG.RGB().Dark() {
		t.Error("a paper-white background was taken as dark")
	}
}

// TestTheFrameIsDrawnAgainstWhatTheTerminalSaid closes the chain that starts with
// the startup probe. A widget that floats one thing over another mixes with what is
// underneath, and a cell nobody coloured has no numbers of its own — so the answer
// has to reach the surface being drawn on, not only the component. Nothing above
// this carries it: the view already knows which terminal it is bound for.
func TestTheFrameIsDrawnAgainstWhatTheTerminalSaid(t *testing.T) {
	h := newHost(t)
	h.ground = grid.Ground{FG: grid.RGBColor(0xC0, 0xCA, 0xF5), BG: grid.RGBColor(0x1A, 0x1B, 0x26)}

	drawnAgainst := make(chan grid.Ground, 1)
	r := startOn(t, h, func(l program.Loop) program.Component {
		return &drawer{loop: l, seen: drawnAgainst}
	})
	r.until("the opening frame", func() bool { return h.frames.size() > 0 })

	if got := <-drawnAgainst; got != h.ground {
		t.Errorf("the frame was drawn against %+v, want what the terminal said", got)
	}
}

// drawer reports the ground the frame it was handed resolves defaults against.
type drawer struct {
	loop program.Loop
	seen chan grid.Ground
}

func (d *drawer) Draw(v grid.View) {
	select {
	case d.seen <- v.Ground():
	default:
	}
	v.Text(0, 0, "ready", grid.Style{})
}

func (d *drawer) Handle(input.Event) bool { return true }

func TestAComponentIsToldWhenTheTerminalNeverSaid(t *testing.T) {
	// There is no safe guess, so the unknown has to be reportable as unknown rather
	// than as the zero colour — which is black, and would silently mean "dark" to
	// everything above it.
	h := newHost(t)

	known, dark := make(chan bool, 1), make(chan bool, 1)
	r := startOn(t, h, func(l program.Loop) program.Component {
		ground := l.Ground()
		known <- !ground.BG.Default()
		dark <- ground.BG.RGB().Dark()
		return &component{text: "ready", consume: true, loop: l}
	})
	r.until("the opening frame", func() bool { return h.frames.size() > 0 })

	if <-known {
		t.Fatal("a host that said nothing was reported as having said something")
	}
	if !<-dark {
		t.Error("the zero colour is not black, so this test asserts nothing")
	}
}

// recorder is a component that keeps what it was handed, so a test can assert on
// which event a reply turned into.
type recorder struct {
	loop    program.Loop
	handled atomic.Int64
	mu      sync.Mutex
	pastes  []string
}

func (r *recorder) Draw(v grid.View) { v.Text(0, 0, "ready", grid.Style{}) }

func (r *recorder) Handle(ev input.Event) bool {
	r.handled.Add(1)
	r.mu.Lock()
	defer r.mu.Unlock()
	if paste, ok := ev.(input.Paste); ok {
		r.pastes = append(r.pastes, paste.Text)
	}
	return true
}

func (r *recorder) pasted() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.pastes)
}

// startRecording runs a program whose component keeps what it was handed.
func startRecording(t *testing.T) (*running, *recorder) {
	t.Helper()
	h := newHost(t)
	rec := &recorder{}
	r := startOn(t, h, func(l program.Loop) program.Component {
		rec.loop = l
		return rec
	})
	r.until("the opening frame", func() bool { return h.frames.size() > 0 })
	return r, rec
}

func TestCopyGoesToTheHost(t *testing.T) {
	// Who does the copying is the host's business. A component asks, and what it
	// asked for is observable without anyone parsing an escape sequence.
	r, rec := startRecording(t)
	if !rec.loop.Copy("hello") {
		t.Fatal("a copy the host accepts was reported as refused")
	}
	if got := r.host.copies(); len(got) != 1 || got[0] != "hello" {
		t.Errorf("the host was given %q, want [hello]", got)
	}
}

func TestCopyReportsAHostThatWillNotTakeIt(t *testing.T) {
	r, rec := startRecording(t)
	r.host.clipMu.Lock()
	r.host.refuse = true
	r.host.clipMu.Unlock()

	if rec.loop.Copy("hello") {
		t.Error("a refused copy was reported as asked for")
	}
	if got := r.host.copies(); len(got) != 0 {
		t.Errorf("the host recorded %q after refusing", got)
	}
}

// TestPasteComesBackAsAPaste is the seam that makes this worth having: a component
// that already inserts what the user pasted needs nothing further to insert what
// they copied somewhere else.
func TestPasteComesBackAsAPaste(t *testing.T) {
	r, rec := startRecording(t)
	r.host.clipMu.Lock()
	r.host.pasteFor = "copied"
	r.host.clipMu.Unlock()

	rec.loop.Paste()
	r.until("the answer to arrive as a paste", func() bool { return len(rec.pasted()) == 1 })
	if got := rec.pasted()[0]; got != "copied" {
		t.Errorf("pasted %q, want %q", got, "copied")
	}
}

func TestPasteThatIsNeverAnsweredIsNotAnError(t *testing.T) {
	// Most terminals refuse to be read, and a refusal has no reply. Asking has to
	// be safe and silent.
	r, rec := startRecording(t)
	rec.loop.Paste()
	r.until("the host to be asked", func() bool { return r.host.timesAsked() == 1 })

	// Followed by something observable, so the absence of a paste is waited for
	// rather than assumed.
	r.host.rune('z')
	r.until("the key after the unanswered request", func() bool { return rec.handled.Load() >= 1 })
	if got := rec.pasted(); len(got) != 0 {
		t.Errorf("an unanswered request produced the paste %q", got)
	}
}

func (h *host) Wheel() input.Wheel { return h.wheel }

func (h *host) Keyboard() (input.KeyboardFlags, bool) { return h.keys, h.saidKeys }

// ReportDirectory does nothing, which is what a host with no terminal to tell has to
// do — and reports no error, because there was nothing to fail.
func (h *host) ReportDirectory(path string) error {
	h.clipMu.Lock()
	defer h.clipMu.Unlock()
	h.directory = path
	return nil
}

func TestAComponentLearnsWhatANotchIs(t *testing.T) {
	// The fact a scroll cannot work out for itself and a component cannot read from
	// anywhere else.
	h := newHost(t)
	h.wheel = input.Wheel{Reports: 1, Rows: 3}

	got := make(chan input.Wheel, 1)
	r := startOn(t, h, func(l program.Loop) program.Component {
		got <- l.Wheel()
		return &component{text: "ready", consume: true, loop: l}
	})
	r.until("the opening frame", func() bool { return h.frames.size() > 0 })

	if want := (input.Wheel{Reports: 1, Rows: 3}); <-got != want {
		t.Errorf("the component was told %+v", want)
	}
}

func TestAComponentLearnsWhatTheKeyboardActuallyDoes(t *testing.T) {
	// The case this exists for: the protocol is live and no release will ever arrive.
	// Nothing in the events says so, so a component that cannot ask cannot choose a
	// different interaction.
	h := newHost(t)
	h.keys, h.saidKeys = input.KeyboardFlags{Flags: input.KittyDisambiguate}, true

	got := make(chan input.KeyboardFlags, 1)
	said := make(chan bool, 1)
	r := startOn(t, h, func(l program.Loop) program.Component {
		flags, ok := l.Keyboard()
		got <- flags
		said <- ok
		return &component{text: "ready", consume: true, loop: l}
	})
	r.until("the opening frame", func() bool { return h.frames.size() > 0 })

	if !<-said {
		t.Fatal("the component was told nothing was known")
	}
	flags := <-got
	if !flags.Has(input.KittyDisambiguate) {
		t.Error("disambiguation was not reported")
	}
	if flags.Has(input.KittyReportEvents) {
		t.Error("releases were reported by a terminal that turned them off")
	}
}

func TestAComponentCanTellTheTerminalWhereItIs(t *testing.T) {
	// The other half of leaving a relative path to the terminal: it can only resolve
	// one against a directory it knows.
	h := newHost(t)
	done := make(chan error, 1)
	r := startOn(t, h, func(l program.Loop) program.Component {
		done <- l.ReportDirectory("/tmp/work")
		return &component{text: "ready", consume: true, loop: l}
	})
	r.until("the opening frame", func() bool { return h.frames.size() > 0 })

	if err := <-done; err != nil {
		t.Fatalf("ReportDirectory: %v", err)
	}
	h.clipMu.Lock()
	defer h.clipMu.Unlock()
	if h.directory != "/tmp/work" {
		t.Errorf("the host was told %q", h.directory)
	}
}

func TestTheBlockIsLeftBehindBeforeAnythingElseGetsTheTerminal(t *testing.T) {
	// An editor, a pager, anything that wants the terminal for itself. The interface
	// is left in the terminal's own output with the cursor below it, so the child
	// starts on a line of its own rather than on top of a block that is no longer
	// being redrawn.
	h, root, done := startInline(t)

	var (
		drawnDuring atomic.Int64
		drawnBy     atomic.Int64
		seenDuring  atomic.Value
	)
	root.hand = func() error {
		seenDuring.Store(h.frames.String())
		before := root.drawn.Load()
		time.Sleep(30 * time.Millisecond)
		drawnDuring.Store(root.drawn.Load() - before)
		drawnBy.Store(root.drawn.Load())
		return nil
	}
	h.send(input.Key{Code: input.Character, Rune: 'e'})

	waitFor(t, done, h, "the terminal to be handed over and taken back", func() bool {
		return root.handErr.Load() != nil
	})
	if err := *root.handErr.Load(); err != nil {
		t.Fatalf("handing the terminal over: %v", err)
	}
	if h.timesHanded() != 1 {
		t.Fatalf("the host was handed the terminal %d times", h.timesHanded())
	}

	// The cursor is shown and the block finished before the child ran, and what was
	// queued had reached the terminal: a frame arriving in the middle of a child's
	// output is a frame drawn over it.
	seen, _ := seenDuring.Load().(string)
	if !strings.HasSuffix(seen, "\x1b[?25h") {
		t.Errorf("the child was given a terminal still holding the block: %q", seen)
	}
	if n := drawnDuring.Load(); n != 0 {
		t.Errorf("the interface drew %d frames while a child owned the terminal", n)
	}

	// And it is drawn again afterwards, in full, because what a child did to the
	// screen is not knowable. Against what it had drawn while the terminal was
	// somebody else's, not against now: the redraw may well have happened already.
	waitFor(t, done, h, "the interface to be drawn again", func() bool {
		return root.drawn.Load() > drawnBy.Load()
	})

	root.loop.Quit()
	if err := <-done; err != nil {
		t.Fatalf("the program ended with %v", err)
	}
}

func TestAnInterfaceCanSayThingsThatAreNotDrawn(t *testing.T) {
	// A window title, a bell, a notification. All three reach somebody who is looking
	// at another window, which is the one thing an interface cannot do by drawing —
	// and all three are the host's, because what they mean depends entirely on what
	// is at the other end.
	r := start(t, nil)
	r.until("the first frame", func() bool { return r.root.drawn.Load() > 0 })

	r.root.loop.SetTitle("building oolong")
	r.root.loop.Bell()
	r.root.loop.Notify("tests passed")
	r.until("what was said to reach the terminal", func() bool {
		title, rang, notified := r.host.said()
		return title == "building oolong" && rang == 1 && len(notified) == 1
	})

	r.root.loop.Quit()
	if err := r.wait(); err != nil {
		t.Fatalf("the program ended with %v", err)
	}
}
