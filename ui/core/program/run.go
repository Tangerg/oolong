package program

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/Tangerg/oolong/ui/core/deadline"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/present"
	"github.com/Tangerg/oolong/ui/core/publication"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/core/term"
	"github.com/Tangerg/oolong/ui/internal/identity"
)

// Run draws the interface until it is asked to stop, its input ends, or the terminal
// fails.
//
// A cancelled context stops the program without being reported as a failure: being
// asked to stop is not one.
func run(ctx context.Context, cfg runConfig) (err error) {
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

	p, buildErr := newProgram(ctx, cfg, session.Host)
	if buildErr != nil {
		return buildErr
	}
	defer p.stop()
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

// newProgram binds a validated transport before invoking application code. That
// order keeps malformed hosts from partially constructing an interface and gives a
// component builder only resources that are ready for use.
func newProgram(ctx context.Context, cfg runConfig, host Host) (*program, error) {
	// The size is asked for once rather than waited for. A program draws before it
	// selects, and a first frame drawn onto a screen of no size is a blank terminal.
	width, height, err := host.Size()
	if err != nil {
		return nil, err
	}
	if err := grid.ValidateTerminalSize(width, height); err != nil {
		return nil, err
	}
	frames := host.Writer()
	if identity.IsNil(frames) {
		return nil, errors.New("program: host returned no frame writer")
	}
	changes := frames.Changes()
	if changes == nil {
		return nil, errors.New("program: frame writer returned no changes channel")
	}
	source := host.Input()
	if identity.IsNil(source) {
		return nil, errors.New("program: host returned no input source")
	}
	events := source.Events()
	if events == nil {
		return nil, errors.New("program: host returned an input source with no event channel")
	}
	p := &program{
		host:          hostServicesFor(host),
		writer:        frames,
		changes:       changes,
		frameInterval: cfg.FrameInterval,
		tasks:         newTaskQueue(),
	}
	built := false
	defer func() {
		if !built {
			p.stop()
		}
	}()
	sessionContext, cancel := context.WithCancel(ctx)
	p.ctx, p.cancel = sessionContext, cancel
	if cfg.Actions > 0 {
		p.tasks.capacity = cfg.Actions
	}
	if cfg.Resources > 0 {
		p.tasks.resourceCapacity = cfg.Resources
	}
	p.environment = environmentFor(p.host)
	if p.frameInterval == 0 {
		p.frameInterval = DefaultFrameInterval
	}
	depth := cfg.Color
	if depth == grid.Auto {
		depth = p.environment.Color()
	}
	if err := p.buildInterface(cfg, width, height, depth); err != nil {
		return nil, err
	}
	p.inputPipe = newInputPipe(sessionContext, p.root, source, events, cfg.Config)
	built = true
	return p, nil
}

func (p *program) buildInterface(cfg runConfig, width, height int, depth grid.Depth) error {
	var target grid.NativeTarget
	if p.host.images != nil {
		target = p.host.images.NativeTarget()
	}
	if cfg.inline {
		p.inline = target.Inline(width, height)
		p.canvas = p.inline
	} else {
		p.canvas = target.Screen(width, height)
	}
	// Construction may publish inline output. Its canvas must already interpret
	// default colours against the same startup facts given to the component.
	p.canvas.SetDepth(depth)
	p.canvas.SetGround(p.environment.Ground())
	if cfg.inline {
		publisher, err := publication.New(publication.Config{Limits: cfg.publication, Width: width, Ground: p.environment.Ground()})
		if err != nil {
			return err
		}
		p.publisher = publisher
	}
	runtime := &Runtime{p: p}
	element, err := cfg.build(runtime)
	if err != nil {
		return fmt.Errorf("program: build interface: %w", err)
	}
	if element.IsZero() {
		return errors.New("program: builder returned an empty element")
	}
	root, err := scene.NewRoot(scene.RootConfig{Element: element})
	if err != nil {
		return fmt.Errorf("program: attach interface: %w", err)
	}
	p.root = root
	return nil
}

// program is one running interface.
type program struct {
	environment Environment
	root        *scene.Root
	ctx         context.Context //nolint:containedctx // The runtime owns the session lifetime through final output settlement.
	cancel      context.CancelFunc
	publisher   *publication.Publisher
	nextTask    uint64
	host        hostServices
	inputPipe   *inputPipe
	canvas      canvas
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

	// tasks separates finite ordinary action capacity from reserved completion
	// slots. Neither admission path waits for work only this owner can consume.
	tasks *taskQueue

	quit atomic.Bool

	// frameFailed prevents shutdown from invoking a painter that has already failed;
	// outputFailed prevents any further frame from being queued after the transport
	// is known to be unusable. Both belong to the single interface owner.
	frameFailed  bool
	outputFailed bool
	// failure is a fatal frame or transport result discovered through an owner-side
	// capability call, which has no return path into the loop of its own. It is the
	// only record of such a result, and [program.takeFailure] is the only way to
	// read one, so it is reported exactly once however the program ends.
	failure error
}

// fail records a fatal result discovered outside the loop's own return path and
// hands err back to the capability's caller. The caller learning of the failure is a
// projection: it may report or recover, but only the record ends the program.
func (p *program) fail(err error) error {
	p.failure = err
	return err
}

// takeFailure consumes the recorded failure. The loop takes it on the next turn
// when there is one, and run takes it when Quit or cancellation means there is not.
// Taking clears it, so the two paths cannot report the same failure twice.
func (p *program) takeFailure() error {
	err := p.failure
	p.failure = nil
	return err
}

func (p *program) run(ctx context.Context) (err error) {
	completed := false
	defer func() {
		if !completed {
			p.frameFailed = true
		}
		p.stopWork()
		if p.publisher != nil {
			p.publisher.CloseAdmission(ErrStopped)
		}
		err = errors.Join(err, p.finish())
	}()
	// Taking the record here is what makes its consumption unconditional. A component
	// that stops the program in the same turn it was handed a failure — the ordinary
	// reaction to one — leaves no next turn for draw to take it in.
	err = errors.Join(p.loop(ctx), p.takeFailure())
	completed = true
	return err
}

func (p *program) loop(ctx context.Context) error {
	// due fires when a frame that was turned away for arriving too soon becomes
	// allowed. Without it the last update of a burst would sit undrawn until something
	// else happened to wake the loop.
	due := deadline.NewTimer()
	defer due.Stop()

	p.present.RequestFull()
	for !p.quit.Load() {
		p.flushNotifications()
		if err := p.takeFailure(); err != nil {
			p.frameFailed = true
			return err
		}
		if err := p.draw(); err != nil && !errors.Is(err, term.ErrQueueFull) {
			p.frameFailed = true
			return err
		}
		p.flushNotifications()
		due.Schedule(p.present.DueAt())
		last, err := p.wait(ctx, due)
		if last || err != nil {
			return err
		}
	}
	return nil
}

func (p *program) wait(ctx context.Context, due *deadline.Timer) (last bool, err error) {
	select {
	case <-ctx.Done():
		return true, nil

	case item, ok := <-p.inputPipe.events:
		if !ok {
			return true, p.inputPipe.err()
		}
		select {
		case p.inputPipe.consumed <- item.bytes:
		case <-p.inputPipe.done:
			// EOF can leave buffered input. Its reader has released its budget,
			// so draining the accepted events needs no further credit delivery.
		}
		if item.repaint {
			return false, p.answerRepaint(item.reply)
		}
		return false, p.handleInput(item.event)

	case <-p.tasks.wake:
		p.runTasks()
		return false, nil

	case _, ok := <-p.changes:
		return false, p.writerChanged(ok)

	case <-due.Channel():
		return false, nil
	}
}

// answerRepaint redraws from nothing and tells whoever asked how it went, before the
// loop ends on the same error: a caller waiting on the reply would otherwise learn
// only that the program had stopped.
func (p *program) answerRepaint(reply func(int, int, error)) error {
	err := ErrInvocationAborted
	defer func() {
		var width, height int
		if err == nil {
			width, height = p.canvas.Size()
		} else {
			p.frameFailed = true
		}
		if reply != nil {
			reply(width, height, err)
		}
	}()
	err = p.repaint()
	return err
}

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
	p.wrote(p.writer.Written())
	if !open {
		p.outputFailed = true
		if err := p.writer.Err(); err != nil {
			p.failPublications(err)
			return err
		}
		err := fmt.Errorf("program: frame writer changes channel closed: %w", term.ErrClosed)
		p.failPublications(err)
		return err
	}
	if err := p.writer.Err(); err != nil {
		p.outputFailed = true
		p.failPublications(err)
		// A terminal that has failed a write does not recover, and an interface that
		// cannot reach its terminal has nothing left to do.
		return err
	}
	return nil
}

func (p *program) apply(task func() bool) {
	if task == nil {
		p.present.RequestBy(time.Now(), p.frameInterval)
		return
	}
	before := p.root.Progress().ModelRevision
	invalidate := task()
	if p.root.Progress().ModelRevision == before {
		if !invalidate {
			return
		}
		p.root.Changed()
	}
	p.present.RequestBy(time.Now(), p.frameInterval)
}

func (p *program) handleInput(ev scene.CapturedInput) error {
	switch e := ev.Input().(type) {
	case input.Resize:
		if err := grid.ValidateTerminalSize(e.Width, e.Height); err != nil {
			return err
		}
		p.canvas.Resize(e.Width, e.Height)
		if p.publisher != nil {
			if err := p.publisher.SetGeometry(e.Width, p.environment.Ground()); err != nil {
				return err
			}
		}
		p.root.InvalidateGeometry()
		p.present.RequestFull()
		return nil
	case input.FocusIn:
		p.root.InvalidateGeometry()
		p.present.RequestFull()
		return nil
	}
	// Consumption controls propagation, not whether handling changed application state.
	p.root.HandleInput(ev)
	p.present.RequestBy(time.Now(), p.frameInterval)
	return nil
}

func (p *program) stopWork() {
	if p.cancel != nil {
		p.cancel()
	}
	if p.tasks != nil {
		p.tasks.stop()
	}
	if p.inputPipe != nil {
		<-p.inputPipe.done
	}
}

func (p *program) flushNotifications() {
	before := p.root.Progress().ModelRevision
	p.root.FlushNotifications()
	changed := p.root.Progress().ModelRevision != before
	if p.publisher != nil && p.publisher.FlushNotifications() {
		changed = true
	}
	if changed {
		p.present.RequestBy(time.Now(), p.frameInterval)
		_ = p.tasks.post(nil)
	}
}

func (p *program) stop() {
	// A retained Runtime still exposes its startup environment. Once cleanup
	// unwinds it must not keep the host, frame buffers or queued input reachable.
	defer func() {
		p.host = hostServices{}
		p.root, p.canvas, p.inline, p.publisher = nil, nil, nil, nil
		p.writer, p.changes, p.inputPipe = nil, nil, nil
		p.ctx, p.cancel = nil, nil
	}()
	p.stopWork()
	if p.publisher != nil {
		p.publisher.Close(ErrStopped)
		defer p.flushClosedPublisher()
	}
	if p.root != nil {
		p.root.Close()
	}
}

// There is no later owner turn after stop. A closed publisher cannot register
// more listeners and claims each callback before running it. Unwinding must
// therefore finish only the uncalled suffix, never replay the interrupted call.
func (p *program) flushClosedPublisher() {
	finished := false
	defer func() {
		if !finished {
			p.flushClosedPublisher()
		}
	}()
	for p.publisher.FlushNotifications() {
	}
	finished = true
}
