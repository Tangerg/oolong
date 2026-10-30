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
	if cfg.inline {
		p.inline = grid.NewInline(width, height)
		p.canvas = p.inline
	} else {
		p.canvas = grid.NewScreen(width, height)
	}
	// Construction may publish inline output. Its canvas must already interpret
	// default colours against the same startup facts given to the component.
	p.canvas.SetDepth(depth)
	p.canvas.SetGround(p.environment.Ground())
	if cfg.inline {
		policy := cfg.publication
		policy.Width, policy.Ground = width, p.environment.Ground()
		publisher, err := publication.New(policy)
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

// canvas is somewhere frames go: a screen of the program's own, or a block in the
// terminal's. The program drives both the same way, and the difference between them
// is entirely in how a frame reaches the wire.
type canvas interface {
	Size() (w, h int)
	Resize(w, h int)
	Invalidate()
	SetDepth(d grid.Depth)
	SetGround(g grid.Ground)
	Frame() grid.View
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
	input       EventSource
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

// takeFailure consumes the recorded failure. Draw takes it on the next loop turn
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
		if err := p.draw(); err != nil {
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
			return true, p.inputPipe.err
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
func (p *program) answerRepaint(reply func(error)) error {
	err := p.repaint()
	if reply != nil {
		reply(err)
	}
	if err != nil {
		p.frameFailed = true
	}
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
		err := errors.New("program: frame writer changes channel closed")
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
	if task() {
		if p.root != nil && p.root.Progress().ModelRevision == before {
			p.root.Changed()
		}
		p.present.RequestBy(time.Now(), p.frameInterval)
	}
}

// handle deals with one terminal event.
//
// Resize and focus are the program's own: the first changes the geometry everything
// else is drawn against, and the second means another program may have written to the
// terminal, so what it is showing can no longer be assumed. Everything else is the
// component's.
func (p *program) handle(ev input.Event) error {
	return p.handleInput(p.root.CaptureInput(ev))
}

func (p *program) handleInput(ev scene.CapturedInput) error {
	switch e := ev.Input.(type) {
	case input.Resize:
		if err := ValidateSize(e.Width, e.Height); err != nil {
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

func (p *program) draw() error {
	if err := p.takeFailure(); err != nil {
		return err
	}
	_, err := p.present.Present(time.Now(), p.compose)
	if errors.Is(err, term.ErrQueueFull) {
		return nil
	}
	return err
}

// compose is the only settlement point for scene geometry, the terminal baseline
// and permanent contributions. No user callback can run between admission and
// the three non-failing commits.
func (p *program) compose(full bool) (uint64, error) {
	if full {
		p.canvas.Invalidate()
	}
	prepared, err := p.root.Prepare(p.canvas.Frame())
	if err != nil {
		return 0, err
	}
	defer prepared.Abort()
	var output *grid.Output
	var batch publication.Batch
	if p.inline != nil {
		batch = p.publisher.Prepare()
		items := batch.Items()
		contents := make([]grid.Frozen, len(items))
		for at, item := range items {
			contents[at] = item.Content
		}
		output, err = p.inline.Prepare(contents...)
	} else {
		screen, ok := p.canvas.(*grid.Screen)
		if !ok {
			return 0, errors.New("program: screen canvas is missing")
		}
		output, err = screen.Prepare()
	}
	if err != nil {
		return 0, fmt.Errorf("program: construct frame: %w", err)
	}
	defer output.Abort()
	seq := uint64(0)
	if output.Len() > 0 {
		seq, err = p.queue(output.Bytes())
		if err != nil {
			return 0, err
		}
	}
	output.Commit()
	prepared.Commit(seq)
	if batch.Len() > 0 {
		p.publisher.Commit(batch, seq)
	}
	p.wrote(p.writer.Written())
	return seq, nil
}

func (p *program) wrote(sequence uint64) {
	p.present.Wrote(sequence)
	if p.root != nil {
		p.root.Wrote(sequence)
	}
	if p.publisher != nil {
		p.publisher.Written(sequence)
	}
}

// queue transfers one non-empty frame and validates the watermark that now owns it.
// Keeping this edge in one method makes ordinary frames and inline settlement obey
// the same publication protocol.
//
// It is also where "no frame after the transport is known unusable" is enforced,
// rather than at each place that publishes one. Spread across call sites, that rule
// held wherever somebody remembered it and nowhere else: a host whose writer broke
// the sequence contract once would be handed the next frame anyway, into a stream
// whose position this program has just said it no longer knows.
func (p *program) queue(frame []byte) (uint64, error) {
	if p.outputFailed {
		return 0, ErrDisplayFailed
	}
	seq, err := p.writer.Queue(frame)
	if err != nil {
		if fatal := p.writer.Err(); fatal != nil {
			err = fatal
		}
		if !errors.Is(err, term.ErrQueueFull) {
			p.outputFailed = true
			p.failPublications(err)
		}
		return 0, err
	}
	if seq == 0 {
		p.outputFailed = true
		if err := p.writer.Err(); err != nil {
			return 0, err
		}
	}
	if seq == 0 || seq <= p.queued {
		p.outputFailed = true
		err := fmt.Errorf("%w: got %d after %d", ErrInvalidFrameSequence, seq, p.queued)
		p.failPublications(err)
		return 0, err
	}
	p.queued = seq
	return seq, nil
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
		err = p.leaveBlock()
	}
	if drainErr := p.writer.Drain(term.DrainGrace); drainErr != nil {
		failure := errors.Join(err, frameDrainError(p.writer, drainErr))
		p.failPublications(failure)
		return failure
	}
	// Failed frames count as drained because retrying a partly written terminal
	// stream is unsound. They are still failures, and may not have reached the event
	// loop's progress case before cancellation or Quit ended the loop.
	p.wrote(p.writer.Written())
	failure := errors.Join(err, p.writer.Err())
	if failure != nil {
		p.failPublications(failure)
	}
	p.flushNotifications()
	return failure
}

// leaveBlock draws the interface one last time and leaves it in the terminal's own
// output, with the cursor below it.
//
// It is what the end of an inline program is made of, and it is also what handing
// the terminal to a child is made of, which is why it is not written inside either:
// both mean "this block is finished with, and whatever writes next starts on a line
// of its own". After it, the block has no position to write relative to, so the next
// frame draws wherever the cursor has ended up.
//
// A construction already known to have failed is not repeated. The pending logical
// frame never existed, so building it again would only run the application's painters
// a second time to arrive at the same failure — while the block last delivered still
// needs its cursor moved below it.
func (p *program) leaveBlock() error {
	var err error
	if !p.frameFailed {
		err = p.renderBlock()
	}
	return errors.Join(err, p.finishBlock())
}

// renderBlock builds one inline frame. It owns the frameFailed transition because
// construction is where it is discovered; no caller has to remember to record it.
func (p *program) renderBlock() error {
	// Shutdown may finish only after the previous frame has left the writer.
	if err := p.writer.Drain(term.DrainGrace); err != nil {
		return frameDrainError(p.writer, err)
	}
	p.wrote(p.writer.Written())
	_, err := p.compose(false)
	if err != nil {
		p.frameFailed = true
	}
	return err
}

func (p *program) finishBlock() error {
	output, err := p.inline.PrepareFinish()
	if err != nil {
		return fmt.Errorf("program: finish inline block: %w", err)
	}
	defer output.Abort()
	if output.Len() > 0 {
		if _, err := p.queue(output.Bytes()); err != nil {
			return err
		}
	}
	output.Commit()
	return nil
}

func (p *program) repaint() error {
	if err := p.writer.Drain(term.DrainGrace); err != nil {
		return frameDrainError(p.writer, err)
	}
	if err := p.writer.Err(); err != nil {
		return err
	}
	p.wrote(p.writer.Written())
	p.present.RequestFull()
	if err := p.draw(); err != nil {
		return err
	}
	return errors.Join(p.writer.Drain(term.DrainGrace), p.writer.Err())
}

// RepaintSource optionally supplies ordered requests for a complete output frame.
// The source must deliver preceding input before delivering its repaint request.
// Each callback runs on the program owner after that frame has settled, with nil
// only when output succeeded. Callbacks must not block. A closed stream ends these
// requests without ending ordinary input. Requests not received before Run ends
// are abandoned; callers must also observe their Run context or completion.
type RepaintSource interface{ Repaints() <-chan func(error) }

func frameDrainError(writer FrameWriter, drainErr error) error {
	return errors.Join(ErrFrameTimeout, drainErr, writer.Err())
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

func (p *program) failPublications(err error) {
	if p.publisher != nil {
		p.publisher.Written(p.writer.Written())
		p.publisher.Fail(err, true)
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
	p.stopWork()
	if p.root != nil {
		p.root.Close()
	}
	if p.publisher != nil {
		p.publisher.Close(ErrStopped)
		p.publisher.FlushNotifications()
	}
}
