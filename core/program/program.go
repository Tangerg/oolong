// Package program runs a terminal interface.
//
// It owns the terminal, the frame schedule, and the one goroutine that is allowed to
// touch the interface's state. It knows nothing about what the interface is for: what
// it drives is a [Component], which draws itself and answers input, and everything a
// component needs from the program it asks for through a [Runtime].
//
// # The concurrency model, in full
//
// One goroutine draws and handles input. Anything that happens elsewhere — a request
// finishing, a file changing, a timer firing — reaches the interface through a
// [Dispatcher] obtained from [Runtime.Dispatcher], and runs there. That is the whole of
// it, and it is why state reached only from that goroutine needs no internal lock.
//
// The program parks when there is nothing to do. It wakes for input, for posted work,
// and for the terminal reporting progress — never on a clock that runs regardless. A
// component that wants a clock starts one with [Runtime.Every], and an interface with
// nothing animating costs nothing.
//
// # The two places an interface can be
//
// A program either takes a screen of its own, which it gives back on the way out, or
// draws in the terminal's own screen as a block with the session's output above it.
// The second is what [Config.Inline] asks for, and it is the difference between a
// program the user enters and leaves and one that is part of their session: what an
// inline interface has finished with is printed with [InlineRuntime.Print] and belongs to
// the terminal from then on — scrollable, selectable, and still there afterwards.
package program

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/layout"
	"github.com/Tangerg/oolong/core/present"
	"github.com/Tangerg/oolong/core/term"
)

// DefaultFrameRate is the fastest a program redraws. A terminal cannot usefully show
// more, and a stream of updates would otherwise ask for a frame each.
const DefaultFrameRate = 16 * time.Millisecond

// ErrFrameTimeout means a frame writer did not account for its pending frames
// before display ownership had to change. The program refuses the transition: a
// late frame would otherwise be written into the next owner's output.
var ErrFrameTimeout = errors.New("program: frame writer did not drain")

// Component is an interface a program can run: it draws itself into the space it is
// given, and says whether it wants an event.
//
// It is handed a view that is already positioned and clipped, so its coordinates are
// its own. An event it does not consume is dropped by the program — a component is the
// root of its own tree and there is nobody above it to pass one on to.
type Component interface {
	Draw(view grid.View)
	Handle(event input.Event) bool
}

// Dispatcher is a copyable, concurrency-safe handle into a running program.
// Its zero value drops work. It deliberately exposes no owner-only operation.
type Dispatcher struct{ tasks *taskQueue }

// Runtime is the program resource owned by the interface goroutine.
//
// It is concrete rather than a provider-defined interface: consumers that need
// only a subset declare that interface where they use it. Background work receives
// only [Runtime.Dispatcher], preserving ownership in the type system. Host features
// are grouped into the concrete [Environment], [Clipboard], [Session] and [Images]
// values rather than flattened into one capability catalogue. The zero value is
// inert; it is safe to embed in an object that has not been attached to a program.
type Runtime struct{ p *program }

// InlineRuntime is a [Runtime] that can publish completed output into terminal
// scrollback. It is only constructed for [Config.Inline], and its zero value is
// inert.
type InlineRuntime struct{ *Runtime }

// Printable is something that can say how tall it is at a width and then draw
// itself into that space.
//
// The interface is defined here where printing consumes it. Any higher-level value
// with the same drawing and measuring behaviour satisfies it without an adapter.
type Printable interface {
	Draw(view grid.View)
	layout.Measurer
}

// EventSource is one ordered input stream and its terminal result.
//
// Events closes after the last event. Once it has closed, Err reports why the
// stream ended: nil means a clean end of input, while a non-nil error is the
// transport failure that ended it. Err must not report [io.EOF] as a failure.
//
// The two methods form one lifecycle. Keeping the result on the source avoids a
// race between an event channel closing and a separate error channel becoming
// readable, and follows the same iteration-then-error shape as a scanner.
type EventSource interface {
	Events() <-chan input.Event
	Err() error
}

// Host is where a program's input comes from and its frames go.
//
// A program opens the real terminal unless it is given one of these. Being able to
// supply it is what lets an interface be driven and inspected in a test, with no
// terminal in sight.
//
// Everything beyond transport is optional. Each independently useful operation is
// represented by a small consumer interface such as [GroundHost], [CopyHost] or
// [NotifyHost], so implementing one never silently depends on implementing its
// neighbours. [ImageHost] is the exception because its transport and geometry form
// one protocol. Absent capabilities receive harmless defaults.
type Host interface {
	// Input is the ordered input stream and the reason it eventually ends.
	Input() EventSource
	// Writer is where frames go. The interface is defined here, where it is used;
	// a host is not coupled to the terminal package's concrete writer.
	Writer() FrameWriter
	// Size is the terminal's size in cells.
	Size() (w, h int, err error)
}

// FrameWriter is the part of a frame queue the program needs.
//
// It is defined by the consumer rather than exposing [term.Writer] through [Host].
// Implementations must preserve queue order, report progress as a watermark and be
// safe for concurrent use. Progress returns one stable channel for the writer's
// lifetime; closing it means the writer has permanently stopped and Err must report
// the cause. Drain returns nil only after every frame accepted before the call has
// either been written or accounted for. [term.Writer] is the standard implementation.
type FrameWriter interface {
	Queue(frame []byte) uint64
	Progress() <-chan struct{}
	Written() uint64
	Err() error
	Drain(timeout time.Duration) error
}

// Config is what a program needs to run.
//
// Exactly one of Root and Inline says what to run, and which one it is decides
// where the interface is drawn: Root takes a screen of its own, Inline draws in the
// terminal's own screen and prints finished output into its scrollback.
type Config struct {
	// Root builds the component to run on a screen of its own. It is given the runtime
	// first, so the component can hold it from the moment it exists. Returning nil is
	// an error.
	Root func(*Runtime) Component

	// Inline builds the component to run as a block in the terminal's own screen,
	// with output that is finished printed above it. Its component is given an
	// [InlineRuntime], which is a [Runtime] that can also print.
	// Returning nil is an error.
	Inline func(*InlineRuntime) Component

	// Terminal says which of the terminal's optional behaviours to ask for. Ignored
	// when Host is set.
	//
	// AltScreen is the program's to decide rather than the caller's, because where
	// frames go is the rendering model and not an input capability: it follows from
	// which of Root and Inline was set. Asking for it alongside Inline is a
	// contradiction and is reported as one.
	Terminal term.Options

	// Color says how much colour the terminal can show. The zero value, [grid.Auto],
	// asks [term.DetectDepth] — which is the one thing in this library that reads
	// its environment rather than making a request and letting it be ignored,
	// because a truecolor sequence a terminal cannot read prints wrong rather than
	// degrading.
	//
	// Setting it is how a program that already knows — from its own configuration,
	// or because it is writing to something that is not a terminal at all — takes
	// that decision back.
	Color grid.Depth

	// Host overrides where input comes from and frames go. Nil opens the real terminal
	// and gives it back on the way out.
	Host Host

	// FrameRate caps how often the interface redraws. Zero uses [DefaultFrameRate].
	FrameRate time.Duration
}

// Run draws the interface until it is asked to stop, its input ends, or the terminal
// fails.
//
// A cancelled context stops the program without being reported as a failure: being
// asked to stop is not one.
func Run(ctx context.Context, cfg Config) (err error) {
	if (cfg.Root == nil) == (cfg.Inline == nil) {
		return errors.New("program: exactly one of Root and Inline is required")
	}
	if cfg.Inline != nil && cfg.Terminal.AltScreen {
		return errors.New("program: an inline interface cannot take the alternate screen")
	}
	opts := cfg.Terminal
	opts.AltScreen = cfg.Root != nil

	host := cfg.Host
	if host == nil {
		terminal, openErr := term.Open(opts)
		if openErr != nil {
			return openErr
		}
		// Giving the terminal back matters more than anything that could go wrong while
		// using it: a terminal left in raw mode is one the user has to close.
		defer func() { err = errors.Join(err, terminal.Close()) }()
		host = terminalHost{Terminal: terminal}
	}

	// The size is asked for once, here, rather than waited for. A program draws before
	// it selects — that is what puts the interface up without the user having to press
	// something — and a first frame drawn onto a screen of no size is a blank terminal.
	width, height, err := host.Size()
	if err != nil {
		return err
	}

	frames := host.Writer()
	if frames == nil {
		return errors.New("program: host returned no frame writer")
	}
	progress := frames.Progress()
	if progress == nil {
		return errors.New("program: frame writer returned no progress channel")
	}
	source := host.Input()
	if source == nil {
		return errors.New("program: host returned no input source")
	}
	events := source.Events()
	if events == nil {
		return errors.New("program: host returned an input source with no event channel")
	}
	p := &program{
		host:      hostServicesFor(host),
		input:     source,
		events:    events,
		writer:    frames,
		progress:  progress,
		frameRate: cfg.FrameRate,
		tasks:     newTaskQueue(),
	}
	if p.frameRate <= 0 {
		p.frameRate = DefaultFrameRate
	}
	defer p.tasks.stop()
	depth := cfg.Color
	if depth == grid.Auto {
		depth = term.DetectDepth()
	}
	if cfg.Inline != nil {
		p.inline = grid.NewInline(width, height)
		p.inline.SetDepth(depth)
		p.canvas = p.inline
		p.root = cfg.Inline(&InlineRuntime{Runtime: &Runtime{p: p}})
	} else {
		screen := grid.NewScreen(width, height)
		screen.SetDepth(depth)
		p.canvas = screen
		p.root = cfg.Root(&Runtime{p: p})
	}
	if p.root == nil {
		return errors.New("program: component builder returned nil")
	}
	// What the terminal draws with and on goes to the canvas, not just to the
	// component: a cell left at the terminal's own colours has no numbers of its own,
	// and a layer floating over one has to mix with something. This is the only place
	// that knows both the answer and the surface, so it is the only place that can
	// join them — see [grid.Ground].
	p.canvas.SetGround(p.host.ground())
	return p.run(ctx)
}

// terminalHost adapts the concrete terminal to the consumer-defined Host without
// making the substrate import this package. Every optional host capability is
// promoted from Terminal; only the frame writer needs an adapter because Go method
// results are not covariant.
type terminalHost struct{ *term.Terminal }

func (h terminalHost) Writer() FrameWriter { return h.Terminal.Writer() }
func (h terminalHost) Input() EventSource  { return terminalInput(h) }

// terminalInput adapts the terminal's concrete input result to EventSource. The
// wrapper keeps term below program in the dependency graph.
type terminalInput struct{ *term.Terminal }

func (i terminalInput) Err() error { return i.InputErr() }

// canvas is somewhere frames go: a screen of the program's own, or a block in the
// terminal's. The program drives both the same way, and the difference between them
// is entirely in how a frame reaches the wire.
type canvas interface {
	Size() (w, h int)
	Resize(w, h int)
	Invalidate()
	SetGround(g grid.Ground)
	Frame() grid.View
	Flush(w io.Writer) error
}

// program is one running interface.
type program struct {
	root   Component
	host   hostServices
	input  EventSource
	events <-chan input.Event
	canvas canvas
	// inline is the canvas again when the interface is drawn in the terminal's own
	// screen, and nil when it has a screen to itself. It is what printing needs and
	// a screen cannot offer.
	inline *grid.Inline
	writer FrameWriter
	// progress is read once and then owned by the event loop. A writer returning a
	// different channel per call would split one watermark into unrelated streams.
	progress <-chan struct{}

	present   present.Presenter
	frameRate time.Duration

	// tasks is the concurrency-safe edge into this goroutine. It is a FIFO plus one
	// wake-up rather than a bounded channel, so the owner can never deadlock behind
	// work only it can consume.
	tasks *taskQueue

	quit atomic.Bool

	// frameFailed prevents shutdown from invoking a painter that has already failed;
	// outputFailed prevents any further frame from being queued after the transport
	// is known to be unusable. Both belong to the single interface owner.
	frameFailed  bool
	outputFailed bool
	// failure is a fatal frame or transport result discovered through an owner-side
	// capability call. The next loop turn returns it before drawing again, so the
	// same failure has one meaning whether it arose in Present or Session.Hand.
	failure error
}

// run is the event loop.
func (p *program) run(ctx context.Context) (err error) {
	// However this ends — asked to stop, input gone, terminal broken — an inline
	// interface has one more frame to draw and a cursor to leave in a sane place.
	defer func() { err = errors.Join(err, p.finish()) }()
	// due fires when a frame that was turned away for arriving too soon becomes
	// allowed. Without it the last update of a burst would sit undrawn until something
	// else happened to wake the loop.
	due := time.NewTimer(0)
	defer due.Stop()
	if !due.Stop() {
		<-due.C
	}
	armed := false

	p.present.RequestFull()
	for !p.quit.Load() {
		if err := p.draw(); err != nil {
			p.frameFailed = true
			return err
		}

		if armed && !due.Stop() {
			<-due.C
		}
		armed = false
		if at, pending := p.present.DueAt(); pending {
			due.Reset(max(time.Until(at), 0))
			armed = true
		}

		select {
		case <-ctx.Done():
			return nil

		case ev, ok := <-p.events:
			if !ok {
				// A clean end closes the session normally. A transport that knows it
				// failed keeps that cause on the source instead of making channel closure
				// indistinguishable from EOF.
				return p.input.Err()
			}
			p.handle(ev)

		case <-p.tasks.wake:
			// Take one snapshot. Work posted while it runs leaves another wake-up, so a
			// producer that never stops cannot keep input out of the select forever.
			for _, task := range p.tasks.take() {
				p.apply(task)
				if p.quit.Load() {
					break
				}
			}

		case _, ok := <-p.progress:
			if !ok {
				p.outputFailed = true
				if err := p.writer.Err(); err != nil {
					return err
				}
				return errors.New("program: frame writer progress channel closed")
			}
			p.present.Wrote(p.writer.Written())
			if err := p.writer.Err(); err != nil {
				p.outputFailed = true
				// A terminal that has failed a write does not recover, and an interface
				// that cannot reach its terminal has nothing left to do.
				return err
			}

		case <-due.C:
			armed = false
		}
	}
	return nil
}

// apply runs one posted task and asks for a frame.
func (p *program) apply(task func()) {
	if task != nil {
		task()
	}
	p.present.RequestBy(time.Now(), p.frameRate)
}

// handle deals with one terminal event.
//
// Resize and focus are the program's own: the first changes the geometry everything
// else is drawn against, and the second means another program may have written to the
// terminal, so what it is showing can no longer be assumed. Everything else is the
// component's.
func (p *program) handle(ev input.Event) {
	switch e := ev.(type) {
	case input.Resize:
		p.canvas.Resize(e.Width, e.Height)
		p.present.RequestFull()
		return
	case input.FocusIn:
		p.present.RequestFull()
		return
	}
	if p.root.Handle(ev) {
		p.present.RequestBy(time.Now(), p.frameRate)
	}
}

// draw renders a frame, if one is owed and the terminal is keeping up.
func (p *program) draw() error {
	if p.failure != nil {
		return p.failure
	}
	_, err := p.present.Present(time.Now(), func(full bool) (uint64, error) {
		if full {
			p.canvas.Invalidate()
		}
		p.root.Draw(p.canvas.Frame())
		return p.flush()
	})
	return err
}

// flush hands the frame to the writer and returns the sequence it was queued under.
//
// The canvas writes into a buffer rather than straight to the terminal, because the
// write has to happen on the writer's goroutine: that is the whole reason there is one.
func (p *program) flush() (uint64, error) {
	var frame frameBuffer
	if err := p.canvas.Flush(&frame); err != nil {
		return 0, fmt.Errorf("program: construct frame: %w", err)
	}
	if len(frame.bytes) == 0 {
		return 0, nil
	}
	return p.writer.Queue(frame.bytes), nil
}

// finish settles what the program leaves behind.
//
// An inline interface's last state is what stays in the terminal, so it is drawn one
// more time and the cursor is left below it — otherwise the shell's next prompt lands
// on top of what the program was showing. The last frame is drawn without asking the
// presenter: pacing is about not drawing more often than a terminal can keep up with,
// and there is no next frame to be too close to. Anything printed but not yet written
// goes out with it, because output the caller asked for must not be lost to the timing
// of the exit.
//
// A program on a screen of its own needs none of that. Giving the screen back takes
// the interface with it, which is what makes that mode simple and this one not.
//
// Both wait for the terminal to catch up, so that Run returning means what the
// program drew has been written. Without it a caller printing its own output next
// would find the program's last frame arriving in the middle of it.
func (p *program) finish() error {
	var err error
	if p.writer.Err() != nil {
		p.outputFailed = true
	}
	if p.inline != nil && !p.outputFailed {
		// A painter failure means the pending logical frame never existed. Repeating
		// the same construction during teardown would only duplicate the failure, but
		// the last successfully delivered block still needs its cursor moved below it.
		if !p.frameFailed {
			err = p.renderBlock()
		}
		err = errors.Join(err, p.finishBlock())
	}
	if drainErr := p.writer.Drain(term.DrainGrace); drainErr != nil {
		return errors.Join(err, frameDrainError(p.writer, drainErr))
	}
	// Failed frames count as drained because retrying a partly written terminal
	// stream is unsound. They are still failures, and may not have reached the event
	// loop's progress case before cancellation or Quit ended the loop.
	return errors.Join(err, p.writer.Err())
}

func frameDrainError(writer FrameWriter, drainErr error) error {
	return errors.Join(ErrFrameTimeout, drainErr, writer.Err())
}

// leaveBlock draws the interface one last time and leaves it in the terminal's own
// output, with the cursor below it.
//
// It is what the end of an inline program is made of, and it is also what handing
// the terminal to a child is made of, which is why it is not written inside either:
// both mean "this block is finished with, and whatever writes next starts on a line
// of its own". After it, the block has no position to write relative to, so the next
// frame draws wherever the cursor has ended up.
func (p *program) leaveBlock() error {
	if err := p.renderBlock(); err != nil {
		return err
	}
	return p.finishBlock()
}

func (p *program) renderBlock() error {
	p.root.Draw(p.inline.Frame())
	_, err := p.flush()
	return err
}

func (p *program) finishBlock() error {
	var tail frameBuffer
	if err := p.inline.Finish(&tail); err != nil {
		return fmt.Errorf("program: finish inline block: %w", err)
	}
	if len(tail.bytes) > 0 {
		p.writer.Queue(tail.bytes)
	}
	return nil
}

// frameBuffer collects one frame's bytes.
type frameBuffer struct{ bytes []byte }

func (f *frameBuffer) Write(b []byte) (int, error) {
	f.bytes = append(f.bytes, b...)
	return len(b), nil
}

// Print publishes a measured drawable above an inline interface.
func (r *InlineRuntime) Print(p Printable) {
	if r == nil || r.Runtime == nil || r.p == nil || r.p.inline == nil || p == nil {
		return
	}
	width, _ := r.p.inline.Size()
	r.p.inline.Print(p.Measure(width), p.Draw)
}

// PrintRows publishes a caller-sized drawing above an inline interface.
func (r *InlineRuntime) PrintRows(rows int, draw func(grid.View)) {
	if r == nil || r.Runtime == nil || r.p == nil || r.p.inline == nil || draw == nil {
		return
	}
	r.p.inline.Print(rows, draw)
}

// Append continues the last published row until draw reports completion.
func (r *InlineRuntime) Append(draw func(grid.View) bool) {
	if r == nil || r.Runtime == nil || r.p == nil || r.p.inline == nil || draw == nil {
		return
	}
	for {
		before := r.room()
		more := false
		r.p.inline.Append(func(v grid.View) { more = draw(v) })
		if !more {
			return
		}
		if r.room() == before && before == 0 {
			// A whole row to itself and nothing drawn into it. No amount of room
			// would help, so asking again is asking forever.
			return
		}
		r.p.inline.Break()
	}
}

// room is how much of the open row has been taken, or zero when the next thing
// published starts a row of its own.
func (r *InlineRuntime) room() int {
	if r == nil || r.Runtime == nil || r.p == nil || r.p.inline == nil {
		return 0
	}
	col, open := r.p.inline.Tail()
	if !open {
		return 0
	}
	return col
}

// Dispatcher returns the concurrency-safe handle for background work.
func (r *Runtime) Dispatcher() Dispatcher {
	if r == nil || r.p == nil {
		return Dispatcher{}
	}
	return Dispatcher{tasks: r.p.tasks}
}

// Refresh requests a frame without changing component state.
func (r *Runtime) Refresh() {
	if r != nil && r.p != nil {
		r.p.tasks.post(nil)
	}
}

// Quit asks the program to stop.
func (r *Runtime) Quit() {
	if r == nil || r.p == nil {
		return
	}
	r.p.quit.Store(true)
	// Wake a parked loop so it can observe the transition. The signal carries no
	// task and coalesces with any wake-up already waiting.
	r.p.tasks.signal()
}

// Every schedules coalesced ticks on the interface goroutine.
func (r *Runtime) Every(d time.Duration, fn func()) (stop func()) {
	if r == nil || r.p == nil || d <= 0 || fn == nil {
		return func() {}
	}
	stopped := make(chan struct{})
	dispatch := r.Dispatcher()
	var pending, cancelled atomic.Bool
	// sync.Once rather than a bool: stop is handed out to a caller who may keep it
	// anywhere, and two goroutines reaching a plain flag at once would both close
	// the channel and one would panic. cancelled is published before the close so a
	// tick already selectable at that instant cannot enqueue one last stale call.
	var once sync.Once
	stop = func() {
		once.Do(func() {
			cancelled.Store(true)
			close(stopped)
		})
	}
	go func() {
		ticker := time.NewTicker(d)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if cancelled.Load() {
					return
				}
				if pending.CompareAndSwap(false, true) {
					dispatch.Post(func() {
						defer pending.Store(false)
						if cancelled.Load() {
							return
						}
						fn()
					})
				}
			case <-stopped:
				return
			case <-r.p.tasks.done:
				return
			}
		}
	}()
	return stop
}
