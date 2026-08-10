package program

import (
	"context"
	"errors"
	"io"
	"sync/atomic"
	"time"

	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/present"
	"github.com/Tangerg/oolong/core/term"
)

// Run draws the interface until it is asked to stop, its input ends, or the terminal
// fails.
//
// A cancelled context stops the program without being reported as a failure: being
// asked to stop is not one.
func Run(ctx context.Context, cfg Config) (err error) {
	if validationErr := cfg.Validate(); validationErr != nil {
		return validationErr
	}
	session, openErr := cfg.openHost()
	if openErr != nil {
		return openErr
	}
	// Giving an owned terminal back matters more than anything that could go wrong
	// while using it: a terminal left in raw mode is one the user has to close.
	defer func() { err = errors.Join(err, session.close()) }()

	p, buildErr := newProgram(cfg, session.Host)
	if buildErr != nil {
		return buildErr
	}
	defer p.tasks.stop()
	return p.run(ctx)
}

// hostSession is a transport and the ownership Run acquired with it. A supplied
// host has nothing to release; a local terminal owns raw mode and must be closed.
type hostSession struct {
	Host
	release func() error
}

func (s hostSession) close() error {
	if s.release == nil {
		return nil
	}
	return s.release()
}

func (c Config) openHost() (hostSession, error) {
	if c.Host != nil {
		return hostSession{Host: c.Host}, nil
	}
	opts := c.Terminal
	opts.AltScreen = c.Root != nil
	terminal, err := term.Open(opts)
	if err != nil {
		return hostSession{}, err
	}
	return hostSession{Host: terminalHost{Terminal: terminal}, release: terminal.Close}, nil
}

// newProgram binds a validated transport before invoking application code. That
// order keeps malformed hosts from partially constructing an interface and gives a
// component builder only resources that are ready for use.
func newProgram(cfg Config, host Host) (*program, error) {
	// The size is asked for once rather than waited for. A program draws before it
	// selects, and a first frame drawn onto a screen of no size is a blank terminal.
	width, height, err := host.Size()
	if err != nil {
		return nil, err
	}
	if err := ValidateSize(width, height); err != nil {
		return nil, err
	}
	frames := host.Writer()
	if frames == nil {
		return nil, errors.New("program: host returned no frame writer")
	}
	changes := frames.Changes()
	if changes == nil {
		return nil, errors.New("program: frame writer returned no changes channel")
	}
	source := host.Input()
	if source == nil {
		return nil, errors.New("program: host returned no input source")
	}
	events := source.Events()
	if events == nil {
		return nil, errors.New("program: host returned an input source with no event channel")
	}
	p := &program{
		host:          hostServicesFor(host),
		input:         source,
		events:        events,
		writer:        frames,
		changes:       changes,
		frameInterval: cfg.FrameInterval,
		tasks:         newTaskQueue(),
	}
	built := false
	defer func() {
		if !built {
			p.tasks.stop()
		}
	}()
	if p.frameInterval == 0 {
		p.frameInterval = DefaultFrameInterval
	}
	depth := cfg.Color
	if depth == grid.Auto {
		depth = term.DetectDepth()
	}
	if err := p.buildInterface(cfg, width, height, depth); err != nil {
		return nil, err
	}
	// What the terminal draws with and on goes to the canvas, not just to the
	// component: a cell left at the terminal's own colours has no numbers of its own,
	// and a layer floating over one has to mix with something. This is the only place
	// that knows both the answer and the surface, so it is the only place that can
	// join them — see [grid.Ground].
	p.canvas.SetGround(p.host.ground())
	built = true
	return p, nil
}

func (p *program) buildInterface(cfg Config, width, height int, depth grid.Depth) error {
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
	return nil
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
	// queued is the last sequence this program received. Other users of the same
	// writer may occupy values between frames; only strict forward movement matters.
	queued uint64
	// progress is read once and then owned by the event loop. A writer returning a
	// different channel per call would split one watermark into unrelated streams.
	changes <-chan struct{}

	present       present.Presenter
	frameInterval time.Duration

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
	due := newFrameTimer()
	defer due.stop()

	p.present.RequestFull()
	for !p.quit.Load() {
		if err := p.draw(); err != nil {
			p.frameFailed = true
			return err
		}

		due.schedule(p.present.DueAt())

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
			if eventErr := p.handle(ev); eventErr != nil {
				return eventErr
			}

		case <-p.tasks.wake:
			p.runTasks()

		case _, ok := <-p.changes:
			if writeErr := p.writerChanged(ok); writeErr != nil {
				return writeErr
			}

		case <-due.channel():
			due.fired()
		}
	}
	return nil
}

// frameTimer owns the drain-before-reset protocol of a reusable timer. Keeping the
// armed bit beside the timer prevents the event loop from having two independent
// accounts of whether a value may still be waiting on its channel.
type frameTimer struct {
	timer *time.Timer
	armed bool
}

func newFrameTimer() *frameTimer {
	timer := time.NewTimer(0)
	if !timer.Stop() {
		<-timer.C
	}
	return &frameTimer{timer: timer}
}

func (t *frameTimer) schedule(at time.Time, pending bool) {
	if t.armed && !t.timer.Stop() {
		<-t.timer.C
	}
	t.armed = false
	if pending {
		t.timer.Reset(max(time.Until(at), 0))
		t.armed = true
	}
}

func (t *frameTimer) channel() <-chan time.Time { return t.timer.C }

func (t *frameTimer) fired() { t.armed = false }

func (t *frameTimer) stop() { t.timer.Stop() }

// runTasks takes one scheduling turn. Work posted while this batch runs leaves
// another wake-up, so a burst already waiting cannot keep input out of the select.
func (p *program) runTasks() {
	for _, task := range p.tasks.take() {
		p.apply(task)
		if p.quit.Load() {
			return
		}
	}
}

func (p *program) writerChanged(open bool) error {
	if !open {
		p.outputFailed = true
		if err := p.writer.Err(); err != nil {
			return err
		}
		return errors.New("program: frame writer changes channel closed")
	}
	p.present.Wrote(p.writer.Written())
	if err := p.writer.Err(); err != nil {
		p.outputFailed = true
		// A terminal that has failed a write does not recover, and an interface that
		// cannot reach its terminal has nothing left to do.
		return err
	}
	return nil
}

// apply runs one posted task and asks for a frame.
func (p *program) apply(task func()) {
	if task != nil {
		task()
	}
	p.present.RequestBy(time.Now(), p.frameInterval)
}

// handle deals with one terminal event.
//
// Resize and focus are the program's own: the first changes the geometry everything
// else is drawn against, and the second means another program may have written to the
// terminal, so what it is showing can no longer be assumed. Everything else is the
// component's.
func (p *program) handle(ev input.Event) error {
	switch e := ev.(type) {
	case input.Resize:
		if err := ValidateSize(e.Width, e.Height); err != nil {
			return err
		}
		p.canvas.Resize(e.Width, e.Height)
		p.present.RequestFull()
		return nil
	case input.FocusIn:
		p.present.RequestFull()
		return nil
	}
	if p.root.Handle(ev) {
		p.present.RequestBy(time.Now(), p.frameInterval)
	}
	return nil
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
