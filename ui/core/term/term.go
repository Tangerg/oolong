// Package term owns OS terminal acquisition, input, output, and restoration so
// higher layers can operate on cells and events without a terminal.
package term

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"

	xterm "golang.org/x/term"

	"github.com/Tangerg/oolong/ui/core/clipboard"
	"github.com/Tangerg/oolong/ui/core/graphics"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/termenv"
)

var ErrNotTerminal = errors.New("term: not a terminal")

const (
	inputEventCapacity = 64
	inputReadCapacity  = 4
)

// Features can cross transport boundaries without carrying screen ownership.
// The zero value requests no optional capabilities.
type Features struct {
	Clipboard clipboard.Config
	// Mouse asks for mouse reporting, including movement and not only clicks.
	Mouse bool
	// Focus asks to be told when the terminal window gains or loses focus, which is
	// what lets a UI stop animating while nobody is looking.
	Focus bool
	// Keyboard is the Kitty keyboard protocol enhancements to request. Use
	// [KeyboardCompatible] for the portable set and add features such as
	// [input.KeyboardReportEvents] only when the application consumes them.
	// Terminals that do not implement the protocol ignore the request.
	Keyboard input.KeyboardFeatures
	// Probe trades a bounded startup round trip for the destination's own
	// color and capability replies; environment hints cannot provide those facts.
	Probe bool
}

// Config is the complete construction state of a terminal session.
//
// Features are requests the driven terminal may or may not support. AltScreen is
// ownership: it decides whether closing the session restores the screen that was
// present before it opened. Keeping the two facts distinct lets adapters share
// feature requests without duplicating that ownership decision.
type Config struct {
	Features Features
	// AltScreen draws on a screen of its own, leaving the user's scrollback as it
	// was. Without it, frames are drawn in place among whatever else is on screen.
	AltScreen bool
	// RestoreCursor is the restoration target when discovery cannot report the
	// entry style. The zero value requests a terminal-specific protocol reset; it
	// cannot promise to restore a user preference. A successful probe uses the
	// actual entry style instead. Remote transports use this explicit target.
	RestoreCursor grid.CursorStyle
}

// Terminal binds raw mode, requested modes, readers, writer, and restoration
// to one session. Close restores resources in reverse acquisition order and is
// idempotent, including after failure. The caller must exclusively own foreground
// terminal input and output: correlation IDs cannot authenticate foreign bytes.
// A Terminal must not be copied.
type Terminal struct {
	in, out     *os.File
	inFD, outFD int
	output      *terminalOutput
	modes       Modes
	oldState    *xterm.State

	// The reader claim begins before prepare, while the session still owns the
	// display. Only a successful release hands the modes and descriptor flags
	// to a child. Serialize physical transitions with Close, never callbacks.
	modeMu    sync.Mutex
	ownsModes bool

	events chan input.Event
	writer *Writer
	// Closing application delivery must release its backpressure while the sole
	// input pump still owns replies needed to settle terminal resources.
	deliveryStopped chan struct{}
	// inputReader owns interruptible input, including the platform's buffered characters.
	// Only the reader may consume it; Close joins that owner before restoring modes.
	inputReader *inputReader
	handed      handover
	// The reader orders this boundary after its earlier bytes and before its
	// next read, even if backpressure outlasted the handover.
	restartInput atomic.Bool
	// said is what the terminal was willing to say about itself, when it was asked.
	// It is written once during Open and only read afterwards.
	said answers
	// clipboard owns OSC 52 encoding and the one live read request. Paste may be
	// called from any goroutine while the pump settles its answer.
	clipboard *clipboard.Channel
	// locale, wheelProfile and imageProtocol are immutable facts of this session.
	// They are resolved once from the environment passed to OpenOn and the answers
	// received from this terminal, so later process-environment changes cannot alter
	// a live terminal's capabilities.
	locale        string
	color         grid.Depth
	wheelProfile  input.Wheel
	imageProtocol graphics.Protocol
	title         title
	// task is progress shown outside the cell grid. It owns its keepalive and
	// pauses it whenever another process owns the terminal.
	task       *taskProgress
	uploads    map[uint32]*imageUpload
	imageOwner graphics.ImageOwner

	resized  chan input.Resize
	resizeMu sync.Mutex
	// The watcher and handover both publish through one resize owner, so the
	// latest accepted measurement alone decides whether another size is new.
	size       dimensions
	readerDone chan struct{}
	stop       chan struct{}
	pumpDone   chan struct{}
	resizeDone chan struct{}

	closeOnce sync.Once
	closeErr  error
	inputErr  error
}

// Open refuses redirected input or output with ErrNotTerminal so callers can
// choose ordinary text instead of writing frames into a pipe.
func Open(cfg Config) (*Terminal, error) {
	return OpenOn(os.Stdin, os.Stdout, cfg, os.LookupEnv)
}

// OpenOn accepts explicit files and their terminal environment so PTY and
// remote sessions do not inherit this process's terminal assumptions. Nil
// env means no environment facts are available.
func OpenOn(
	in, out *os.File,
	cfg Config,
	env termenv.Lookup,
) (*Terminal, error) {
	// Resolve every caller-provided environment fact before raw mode, files or
	// readers need cleanup. A lookup may panic or leave its goroutine.
	environment := env.Memoized()
	modes, err := cfg.Modes(environment)
	if err != nil {
		return nil, err
	}
	if in == nil {
		return nil, errors.New("term: input file is required")
	}
	if out == nil {
		return nil, errors.New("term: output file is required")
	}
	inFD, err := checkedTerminalFD(in, "input")
	if err != nil {
		return nil, err
	}
	outFD, err := checkedTerminalFD(out, "output")
	if err != nil {
		return nil, err
	}

	t := newTerminal(in, out, cfg, modes, environment)
	t.inFD, t.outFD = inFD, outFD
	oldState, err := xterm.MakeRaw(inFD)
	if err != nil {
		return nil, fmt.Errorf("term: enter raw mode: %w", err)
	}
	t.oldState = oldState
	if err := t.takeOver(); err != nil {
		return nil, err
	}
	if err := t.start(cfg); err != nil {
		return nil, err
	}
	return t, nil
}

func checkedTerminalFD(file *os.File, role string) (int, error) {
	// File.Fd can switch a pollable descriptor back to blocking, even when
	// this file is rejected and no session will ever restore its flags.
	raw, err := file.SyscallConn()
	if err != nil {
		return 0, err
	}
	var fd int
	var terminal bool
	if err := raw.Control(func(value uintptr) {
		fd = int(value)
		terminal = xterm.IsTerminal(fd)
	}); err != nil {
		return 0, err
	}
	if !terminal {
		return 0, fmt.Errorf("%w: %s is fd %d", ErrNotTerminal, role, fd)
	}
	return fd, nil
}

func newTerminal(
	in, out *os.File,
	cfg Config,
	modes Modes,
	env termenv.Lookup,
) *Terminal {
	return &Terminal{
		in:              in,
		out:             out,
		modes:           modes,
		events:          make(chan input.Event, inputEventCapacity),
		deliveryStopped: make(chan struct{}),
		resized:         make(chan input.Resize, 1),
		stop:            make(chan struct{}),
		readerDone:      make(chan struct{}),
		pumpDone:        make(chan struct{}),
		resizeDone:      make(chan struct{}),
		task:            newTaskProgress(),
		clipboard:       clipboard.New(cfg.Features.Clipboard),
		locale:          DetectLocale(env),
		color:           localDepth(env),
		wheelProfile:    input.WheelFor(env),
		imageProtocol:   graphics.Detect(env),
	}
}

// takeOver acquires the output-side resources in order. Any failure rolls back the
// complete prefix already acquired, leaving start to run only for a usable session.
func (t *Terminal) takeOver() error {
	output, err := newTerminalOutput(t.out)
	if err != nil {
		return errors.Join(err, xterm.Restore(t.inFD, t.oldState))
	}
	t.output = output
	// Acquire everything that can fail before entering terminal modes. Rollback
	// never needs to send restoration through a writer it has already stopped.
	inputReader, err := newInputReader(t.inFD)
	if err != nil {
		return t.abortOpen(err)
	}
	t.inputReader = inputReader
	if err := t.output.SetWriteDeadline(time.Now().Add(DrainGrace)); err != nil {
		return t.abortOpen(err)
	}
	defer func() { _ = t.output.SetWriteDeadline(time.Time{}) }()
	t.ownsModes = true // Even a failed write may have entered a prefix of the modes.
	if _, err := t.output.WriteString(t.modes.Enter()); err != nil {
		return t.abortOpen(fmt.Errorf("term: take over the terminal: %w", err))
	}
	return nil
}

// Every started worker belongs to Close, including when discovery fails. This
// keeps startup rollback on the same join-and-restore path as normal shutdown.
func (t *Terminal) start(cfg Config) error {
	// Subscribed before the size is measured, and for that reason: a change between
	// the measurement and the subscription is one nothing would ever hear about.
	// See [resizeSource].
	resizes := subscribeResize()

	// The size is delivered as an event rather than left to be asked for, so a
	// session learns its size the same way it learns about every later change. No
	// watcher is running yet, so this is the one write to size that needs no lock;
	// the event waits for the pump, which alone writes the event channel.
	var early []input.Event
	if w, h, err := t.Size(); err == nil {
		t.size = knownDimensions(w, h)
		early = append(early, input.Resize{Width: w, Height: h})
	}

	raw := make(chan readResult, inputReadCapacity)
	go t.read(raw)

	// Asking has to happen here, between the reader starting and the pump starting.
	// A terminal has exactly one reader; asking any earlier means nothing is
	// listening for the answer, and any later means two readers race for it.
	//
	// The decoder is this session's for the whole of it. Startup input is ordinary
	// input — the same timing, the same escape grace — so the probe and the pump read
	// through one stream rather than two agreeing decoders.
	stream := input.NewStream(input.StreamConfig{Clipboard: t.clipboard})
	var end *readResult
	var probeErr error
	if cfg.Features.Probe {
		// One budget covers asking and being answered; the probe holds the transport
		// to the same instant.
		pr := &probe{
			raw: raw, out: t.output, stream: stream,
			deadline: time.Now().Add(answerGrace),
		}
		t.said, probeErr = pr.run()
		if cursor, ok := t.said.cursor.Get(); ok {
			t.modes = t.modes.restoringCursor(cursor)
		}
		early = append(early, pr.early...)
		end = pr.end
	}
	// The writer takes the byte stream only once the probe has finished writing
	// to it directly, so the stream has exactly one owner at every moment.
	t.writer = NewWriter(WriterConfig{Destination: t.output})
	go t.task.run(t.stop, t.writer.Queue)
	name, _ := t.said.name.Get()
	t.wheelProfile = t.wheelProfile.Negotiated(name)
	t.imageProtocol = t.said.graphics(t.imageProtocol)

	p := &pump{
		raw: raw, end: end, resized: t.resized, stop: t.stop,
		out: t.events, deliveryStopped: t.deliveryStopped,
		stream: stream, early: early, consume: t.takeImageReply,
	}
	t.startResizeWatcher(resizes)
	go func() {
		defer close(t.pumpDone)
		t.inputErr = p.run()
		t.cancelUploads(t.inputErr)
		close(t.events)
	}()
	if probeErr != nil {
		return errors.Join(fmt.Errorf("term: probe terminal: %w", probeErr), t.Close())
	}
	return nil
}

// abortOpen rolls back every resource acquired before a Terminal became usable.
// Acquisition grows in one direction; every failure leaves through this one edge so
// adding a resource cannot create a new path that forgets an older one.
func (t *Terminal) abortOpen(cause error) error {
	errs := []error{cause}
	if t.ownsModes {
		errs = append(errs, t.giveBack()...)
	} else {
		errs = append(errs, xterm.Restore(t.inFD, t.oldState))
	}
	if t.inputReader != nil {
		t.inputReader.close()
	}
	errs = append(errs, t.output.active(false), t.output.Close())
	return errors.Join(errs...)
}

// Events is the terminal's input, closed when the input ends or the session does.
func (t *Terminal) Events() <-chan input.Event { return t.events }

// InputErr reports why [Terminal.Events] closed.
//
// It must be called only after Events has closed. A clean input end or session stop
// reports nil unless a read failure was already observed. Such failures remain
// visible when event delivery is interrupted. The channel close synchronizes the
// pump's write with this read.
func (t *Terminal) InputErr() error { return t.inputErr }

func (t *Terminal) Writer() FrameSink { return t.writer.Frames() }

// Ground preserves unknown colors as terminal defaults. Blending needs known
// RGB values for both colors; guessing them can make text unreadable on a
// different terminal theme.
func (t *Terminal) Ground() grid.Ground { return t.said.ground }

// Attributes exposes the terminal's claims, including sixel support. These
// claims are negotiation evidence rather than proof that rendering will succeed.
func (t *Terminal) Attributes() (input.DeviceAttributes, bool) {
	return t.said.attributes.Get()
}

// Copy addresses the user's terminal clipboard, which may be remote from this
// process. Admission is serialized with frames so OSC bytes cannot split one.
// A nil error means queued: terminals may refuse clipboard access silently.
// Oversized text is refused with [clipboard.ErrTooLarge].
func (t *Terminal) Copy(text string) error {
	seq, err := t.clipboard.Copy(clipboard.System, text)
	if err != nil {
		return err
	}
	_, err = t.writer.Queue([]byte(seq))
	return err
}

// ReadClipboard waits outside terminal mode locks. Cancellation ends delivery;
// an unanswered OSC 52 query still owns its protocol slot across handover.
func (t *Terminal) ReadClipboard(ctx context.Context) (string, error) {
	return t.clipboard.Read(ctx, clipboard.System, func(sequence string) error {
		_, err := t.writer.Queue([]byte(sequence))
		return err
	})
}

// Name is the name the terminal gave in reply to the startup probe, and whether
// it gave one. It never comes from the environment, which may describe an outer
// or remote terminal. Its spelling is terminal-defined.
func (t *Terminal) Name() (string, bool) { return t.said.name.Get() }

// Version is the number a terminal gives when it will not give a name, and whether it
// gave one. See [input.DeviceVersion] for why the numbers are not interpreted here.
func (t *Terminal) Version() (input.DeviceVersion, bool) {
	return t.said.version.Get()
}

// Keyboard reports the startup probe's accepted-feature snapshot. Consumers
// cannot infer release-event support from presses alone. The boolean requires
// Features.Probe and a matching reply; later protocol-family observations belong
// to the input decoder and do not advance this startup snapshot.
func (t *Terminal) Keyboard() (input.KeyboardFeatures, bool) {
	return t.said.keyboard.Get()
}

// Locale is the terminal environment's character locale. It is the explicit
// OpenOn environment's answer, not the process environment's, and an empty string
// means none was supplied.
func (t *Terminal) Locale() string { return t.locale }

func (t *Terminal) Color() grid.Depth { return t.color }

// Wheel is the batching for the terminal's own reported name when it gave one,
// and otherwise the one its environment suggests.
func (t *Terminal) Wheel() input.Wheel {
	return t.wheelProfile
}

// Close attempts every restoration step even after failure: output errors
// must not leave raw mode active for the next terminal owner. If pending image
// cleanup cannot settle within DrainGrace, it returns ErrImageCleanupIncomplete
// without guessing which physical allocation the next owner may inherit.
func (t *Terminal) Close() error {
	t.closeOnce.Do(func() {
		t.modeMu.Lock()
		t.writer.CloseAdmission()
		t.modeMu.Unlock()
		close(t.deliveryStopped)
		t.clipboard.Interrupt()
		t.cancelUploads(ErrClosed)
		uploadErr := t.awaitUploads()
		close(t.stop)
		// Wake both kernel waits and handover parking so shutdown can join the
		// reader without requiring input or a child to return.
		t.inputReader.wake()
		// A wake request is not a completed read. Modes and input belong to this
		// session until the reader has actually stopped consuming the descriptor.
		<-t.readerDone
		<-t.resizeDone
		<-t.task.done

		t.closeErr = errors.Join(uploadErr, t.closeOutput())
		// The pump owns the event channel, so it has to have finished before anyone
		// could observe the channel closed.
		<-t.pumpDone
	})
	return t.closeErr
}

// closeOutput releases only the display the session still owns. A child may
// close its parent session after changing its own modes or descriptor flags.
func (t *Terminal) closeOutput() error {
	t.modeMu.Lock()
	defer t.modeMu.Unlock()
	defer t.imageOwner.RevokeAll()
	defer t.revokeUploads()
	// One deadline bounds the queued frames, image release and leave sequences, so
	// raw mode is still restored when the display stops draining. It is restored
	// last, in reverse acquisition order: cooked mode while mouse, paste and screen
	// modes are still on would let a keystroke raise a signal or echo into the
	// display before the leave sequence reaches it.
	errs := []error{t.output.SetWriteDeadline(time.Now().Add(DrainGrace))}
	if t.ownsModes {
		errs = append(errs, t.releaseImages())
		_, cleanupErr := t.writer.QueueControl([]byte(t.leaveSequence()))
		errs = append(errs, cleanupErr)
	}
	errs = append(errs, t.writer.Close())
	<-t.writer.loopDone
	errs = append(errs, t.writer.Err())
	var flagsErr error
	if t.ownsModes {
		flagsErr = t.output.active(false)
		if err := xterm.Restore(t.inFD, t.oldState); err != nil {
			errs = append(errs, fmt.Errorf("term: leave raw mode: %w", err))
		}
	}
	if err := errors.Join(flagsErr, t.output.Close()); err != nil {
		errs = append(errs, fmt.Errorf("term: give the terminal back: %w", err))
	}
	return errors.Join(errs...)
}

func (t *Terminal) leaveSequence() string { return t.task.leave() + t.title.leave() + t.modes.Leave() }

// giveBack restores modes during startup rollback or a handover. The caller owns
// an output deadline; Close instead queues these sequences behind pending frames.
func (t *Terminal) giveBack() []error {
	var errs []error
	// Session metadata is cleared before modes, so neither a task nor a title leaks
	// into the next owner.
	errs = append(errs, t.releaseImages())
	if err := t.control(t.leaveSequence()); err != nil {
		errs = append(errs, fmt.Errorf("term: give the terminal back: %w", err))
	}
	if err := xterm.Restore(t.inFD, t.oldState); err != nil {
		errs = append(errs, fmt.Errorf("term: leave raw mode: %w", err))
	}
	return errs
}

// control joins the session writer once it exists. Before that, during startup
// and its rollback, nothing else owns the byte stream and the write is direct.
func (t *Terminal) control(value string) error {
	if t.writer == nil {
		_, err := t.output.WriteString(value)
		return err
	}
	if _, err := t.writer.QueueControl([]byte(value)); err != nil {
		return err
	}
	return errors.Join(t.writer.Drain(DrainGrace), t.writer.Err())
}

const readChunkBytes = 4096
