// Package term owns OS terminal acquisition, input, output, and restoration so
// higher layers can operate on cells and events without a terminal.
package term

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	xterm "golang.org/x/term"

	"github.com/Tangerg/oolong/ui/core/ansi"
	"github.com/Tangerg/oolong/ui/core/clipboard"
	"github.com/Tangerg/oolong/ui/core/graphics"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
)

var ErrNotTerminal = errors.New("term: not a terminal")

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
	// waker owns interruptible input, including the platform's buffered characters.
	// Only the reader may consume it; Close joins that owner before restoring modes.
	waker  *waker
	handed handover
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
// lookup means no environment facts are available.
func OpenOn(
	in, out *os.File,
	cfg Config,
	lookup func(string) (string, bool),
) (*Terminal, error) {
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

	// Resolve every caller-provided environment fact before raw mode, files or
	// readers need cleanup. A lookup may panic or leave its goroutine.
	t := newTerminal(in, out, cfg, startupEnvironment(lookup))
	t.inFD, t.outFD = inFD, outFD
	oldState, err := xterm.MakeRaw(inFD)
	if err != nil {
		return nil, fmt.Errorf("term: enter raw mode: %w", err)
	}
	t.oldState = oldState
	if err := t.takeOver(); err != nil {
		return nil, err
	}
	t.start(cfg)
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

// Capabilities share environment keys. Read each key from the provider once
// without maintaining a separate list of which keys the detectors understand.
func startupEnvironment(lookup func(string) (string, bool)) func(string) (string, bool) {
	if lookup == nil {
		return nil
	}
	type fact struct {
		value string
		set   bool
	}
	facts := make(map[string]fact)
	return func(name string) (string, bool) {
		f, ok := facts[name]
		if !ok {
			f.value, f.set = lookup(name)
			facts[name] = f
		}
		return f.value, f.set
	}
}

func newTerminal(
	in, out *os.File,
	cfg Config,
	lookup func(string) (string, bool),
) *Terminal {
	return &Terminal{
		in:            in,
		out:           out,
		modes:         cfg.Modes(lookup),
		events:        make(chan input.Event, 64),
		resized:       make(chan input.Resize, 1),
		stop:          make(chan struct{}),
		readerDone:    make(chan struct{}),
		pumpDone:      make(chan struct{}),
		resizeDone:    make(chan struct{}),
		task:          newTaskProgress(),
		clipboard:     clipboard.New(cfg.Features.Clipboard),
		locale:        DetectLocale(lookup),
		color:         DetectDepth(lookup),
		wheelProfile:  input.WheelFor(lookup, ""),
		imageProtocol: graphics.Detect(lookup, "", false),
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
	waker, err := newWaker(t.inFD)
	if err != nil {
		return t.abortOpen(err)
	}
	t.waker = waker
	if err := t.output.SetWriteDeadline(time.Now().Add(DrainGrace)); err != nil {
		return t.abortOpen(err)
	}
	defer func() { _ = t.output.SetWriteDeadline(time.Time{}) }()
	t.ownsModes = true // Even a failed write may have entered a prefix of the modes.
	if _, err := t.output.WriteString(t.modes.Enter()); err != nil {
		return t.abortOpen(fmt.Errorf("term: take over the terminal: %w", err))
	}

	t.writer = NewWriter(WriterConfig{Destination: t.output})
	go t.task.run(t.stop, t.writer.Queue)
	return nil
}

// start activates the input side after acquisition can no longer fail. Goroutines
// are deliberately last: no partially opened Terminal is returned, and no rollback
// path has to coordinate workers that escaped into the session.
func (t *Terminal) start(cfg Config) {
	// Subscribed before the size is measured, and for that reason: a change between
	// the measurement and the subscription is one nothing would ever hear about.
	// See [resizeSource].
	resizes := subscribeResize()

	// The size is delivered as an event rather than left to be asked for, so a
	// session learns its size the same way it learns about every later change. No
	// watcher is running yet, so this is the one write to size that needs no lock.
	if w, h, err := t.Size(); err == nil {
		t.size = knownDimensions(w, h)
		t.events <- input.Resize{Width: w, Height: h}
	}

	raw := make(chan readResult, 4)
	go t.read(raw)

	// Asking has to happen here, between the reader starting and the pump starting.
	// A terminal has exactly one reader; asking any earlier means nothing is
	// listening for the answer, and any later means two readers race for it.
	//
	// The decoder is this session's for the whole of it. Startup input is ordinary
	// input — the same timing, the same escape grace — so the probe and the pump read
	// through one stream rather than two agreeing decoders.
	stream := input.NewStream(input.StreamConfig{Clipboard: t.clipboard})
	var early []input.Event
	var end *readResult
	if cfg.Features.Probe {
		// One budget covers asking and being answered; the probe holds the transport
		// to the same instant.
		pr := &probe{
			raw: raw, out: t.output, stream: stream,
			deadline: time.Now().Add(answerGrace),
		}
		t.said = pr.run()
		early = pr.early
		end = pr.end
	}
	t.wheelProfile = t.wheelProfile.Negotiated(t.identity())
	t.imageProtocol = t.said.graphics(t.imageProtocol)

	p := &pump{
		raw: raw, end: end, resized: t.resized, stop: t.stop,
		out: t.events, stream: stream, early: early, consume: t.takeImageReply,
	}
	t.startResizeWatcher(resizes)
	go func() {
		defer close(t.pumpDone)
		t.inputErr = p.run()
		t.cancelUploads(t.inputErr)
		close(t.events)
	}()
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
	if t.waker != nil {
		t.waker.close()
	}
	errs = append(errs, t.output.active(false), t.output.Close())
	return errors.Join(errs...)
}

// Events is the terminal's input, closed when the input ends or the session does.
func (t *Terminal) Events() <-chan input.Event { return t.events }

// InputErr reports why [Terminal.Events] closed.
//
// It must be called only after Events has closed. A clean end of input and a session
// stopped by [Terminal.Close] report nil; another read failure is preserved as its
// cause. The channel close synchronizes the pump's write with this read.
func (t *Terminal) InputErr() error { return t.inputErr }

func (t *Terminal) Writer() FrameSink { return t.writer.Frames() }

// Ground preserves unknown colors as terminal defaults. Blending needs known
// RGB values for both colors; guessing them can make text unreadable on a
// different terminal theme.
func (t *Terminal) Ground() grid.Ground {
	var g grid.Ground
	if t.said.hasFg {
		g.FG = grid.RGBColor(t.said.foreground.R, t.said.foreground.G, t.said.foreground.B)
	}
	if t.said.hasBg {
		g.BG = grid.RGBColor(t.said.background.R, t.said.background.G, t.said.background.B)
	}
	return g
}

// Attributes exposes the terminal's claims, including sixel support. These
// claims are negotiation evidence rather than proof that rendering will succeed.
func (t *Terminal) Attributes() (input.DeviceAttributes, bool) {
	return t.said.attributes, t.said.hasAttrs
}

// Copy addresses the user's terminal clipboard, which may be remote from this
// process. Admission is serialized with frames so OSC bytes cannot split one.
// Success means queued: terminals may refuse clipboard access silently. Oversized
// text is refused according to clipboard.MaxPayload.
func (t *Terminal) Copy(text string) bool {
	seq, ok := t.clipboard.Copy(clipboard.System, text)
	if !ok {
		return false
	}
	_, err := t.writer.Queue([]byte(seq))
	return err == nil
}

// ReadClipboard waits outside terminal mode locks. Cancellation ends delivery;
// an unanswered OSC 52 query still owns its protocol slot across handover.
func (t *Terminal) ReadClipboard(ctx context.Context) (string, error) {
	return t.clipboard.Read(ctx, clipboard.System, func(sequence string) error {
		_, err := t.writer.Queue([]byte(sequence))
		return err
	})
}

// Name prefers the destination's direct reply over environment hints that may
// describe an outer or remote terminal. Its spelling is terminal-defined.
func (t *Terminal) Name() (string, bool) { return t.said.name, t.said.hasName }

// Version is the number a terminal gives when it will not give a name, and whether it
// gave one. See [input.DeviceVersion] for why the numbers are not interpreted here.
func (t *Terminal) Version() (input.DeviceVersion, bool) {
	return t.said.version, t.said.hasVersion
}

// Keyboard reports the startup probe's accepted-feature snapshot. Consumers
// cannot infer release-event support from presses alone. The boolean requires
// Features.Probe and a matching reply; later protocol-family observations belong
// to the input decoder and do not advance this startup snapshot.
func (t *Terminal) Keyboard() (input.KeyboardFeatures, bool) {
	return t.said.keyboard, t.said.hasKeyboard
}

// Locale is the terminal environment's character locale. It is the explicit
// OpenOn environment's answer, not the process environment's, and an empty string
// means none was supplied.
func (t *Terminal) Locale() string { return t.locale }

func (t *Terminal) Color() grid.Depth { return t.color }

func (t *Terminal) identity() string {
	if t.said.hasName {
		return t.said.name
	}
	return ""
}

// Wheel uses negotiated identity before environment hints, as Name does.
func (t *Terminal) Wheel() input.Wheel {
	return t.wheelProfile
}

// Graphics is the richest way this terminal will take an image.
//
// An answered direct-transfer probe takes precedence over terminal name hints. A
// session that did not ask — see [Features.Probe] — gets the answer
// [graphics.Detect] derives from its environment alone.
func (t *Terminal) Graphics() graphics.Protocol {
	return t.imageProtocol
}

// ReportDirectory lets the terminal resolve relative paths in output after
// the program changes directory. The path becomes absolute; empty uses the
// process working directory. A nil error means queued with the frames, while
// Writer reports delivery.
func (t *Terminal) ReportDirectory(path string) error {
	if path == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("term: report the directory: %w", err)
		}
		path = cwd
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("term: report the directory: %w", err)
	}
	// The host is part of the form and is what tells a terminal on the other end of an
	// ssh connection that the path is not one of its own. A machine that cannot name
	// itself reports none, which is the form's own way of saying "wherever this is".
	host, _ := os.Hostname()
	location := (&url.URL{
		Scheme: "file",
		Host:   host,
		Path:   filepath.ToSlash(abs),
	}).String()
	if _, err := t.writer.Queue([]byte("\x1b]7;" + location + "\x1b\\")); err != nil {
		return err
	}
	return nil
}

// Size is the terminal's size in cells.
func (t *Terminal) Size() (w, h int, err error) {
	return xterm.GetSize(t.outFD)
}

func (t *Terminal) NativeTarget() grid.NativeTarget { return t.imageOwner.NativeTarget() }

// CellSize can remain unknown because cell counts do not determine pixel
// dimensions. Image fitting must respect the boolean instead of inventing an
// aspect ratio.
func (t *Terminal) CellSize() (image.Point, bool) {
	return cellSize(t.outFD)
}

// ReleaseImage queues deletion of transmitted image data and all its placements.
// The caller must stop drawing the image before releasing it.
func (t *Terminal) ReleaseImage(resource graphics.Resource) error {
	t.modeMu.Lock()
	defer t.modeMu.Unlock()
	return t.imageOwner.Release(resource, func(img graphics.Image) error {
		return deleteImage(img, t.writer.Queue)
	})
}

func deleteImage(img graphics.Image, queue func([]byte) (uint64, error)) error {
	var payload bytes.Buffer
	if err := img.Release(&payload); err != nil {
		return err
	}
	_, err := queue(payload.Bytes())
	return err
}

// Never delete by ID after the child takes over. Draining between admissions
// keeps cleanup inside the writer's fixed control budget for any image count.
func (t *Terminal) releaseImages() error {
	if err := t.releasePendingImages(); err != nil {
		return err
	}
	return t.imageOwner.ReleaseAll(func(img graphics.Image) error {
		if err := t.writer.Drain(DrainGrace); err != nil {
			return err
		}
		return deleteImage(img, t.writer.QueueControl)
	})
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
		t.clipboard.Interrupt()
		t.cancelUploads(ErrClosed)
		uploadErr := t.awaitUploads()
		close(t.stop)
		// Wake both kernel waits and handover parking so shutdown can join the
		// reader without requiring input or a child to return.
		t.waker.wake()
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
	var errs []error
	if t.ownsModes {
		// Raw mode must settle independently of output backpressure. The same
		// deadline covers queued frames and the final restore sequences.
		if err := xterm.Restore(t.inFD, t.oldState); err != nil {
			errs = append(errs, fmt.Errorf("term: leave raw mode: %w", err))
		}
	}
	errs = append(errs, t.output.SetWriteDeadline(time.Now().Add(DrainGrace)))
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

// control joins the session writer once it exists. Startup rollback is the only
// direct write: no writer has taken ownership of the byte stream at that point.
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

// read is the sole input consumer. The platform atom owns waiting and reading
// together: console readiness can describe records that produce no characters.
// Returning to this loop after each bounded read lets handover and shutdown claim
// input without cancelling a kernel read or leaving a second consumer behind.
func (t *Terminal) read(raw chan<- readResult) {
	defer close(t.readerDone)
	defer t.waker.close()
	for {
		// Bytes already removed from the terminal precede the handover boundary.
		// Drain that bounded platform buffer before confirming that input parked;
		// only then may the child own the next kernel read.
		if !t.waker.buffered() {
			t.handed.park(t.stop)
		}
		select {
		case <-t.stop:
			return
		default:
		}
		if t.restartInput.Swap(false) {
			t.waker.reset()
			if !t.sendRead(raw, readResult{restart: true}) {
				return
			}
		}

		data, err := t.waker.read(t.in)
		at := time.Now()
		if len(data) > 0 || err != nil {
			if !t.sendRead(raw, readResult{data: data, at: at, err: err}) {
				return
			}
		}
		if err != nil {
			return
		}
		select {
		case <-t.stop:
			return
		default:
		}
	}
}

func (t *Terminal) sendRead(raw chan<- readResult, result readResult) bool {
	select {
	case raw <- result:
		return true
	case <-t.stop:
		return false
	}
}

// SetTitle retains the title-stack restoration obligation alongside writer
// admission. Peers may omit title-stack support, so restoration is best-effort.
// Titles too large to restate after handover return ErrPayloadTooLarge.
func (t *Terminal) SetTitle(s string) error {
	return t.title.to(s, len(t.modes.Enter()), t.writer.Queue)
}

// Bell delegates attention policy to the user's terminal configuration.
func (t *Terminal) Bell() error { _, err := t.writer.Queue([]byte{ansi.Bell}); return err }

// Notify is supplementary: unsupported peers may ignore it without a reply.
// Numeric-leading text receives a bullet so OSC 9 peers cannot interpret it as
// a control command in their shared notification namespace.
func (t *Terminal) Notify(text string) error {
	_, err := t.writer.Queue([]byte(notification(text)))
	return err
}

// Hand parks the reader before restoring modes so the child owns every input
// byte. It runs synchronously on the caller's goroutine; the caller must keep
// other output producers idle until return. Ownership is restored even if run
// panics, unless the session was closed from run.
//
// prepare runs after the handover claim while interface modes remain active.
// A prepare error leaves modes unchanged and skips run. Failure to park within
// the grace period wraps os.ErrDeadlineExceeded and also skips run. Nested or
// concurrent calls return ErrHandoverActive; closed sessions return ErrClosed.
// Unsupported input handover returns errors.ErrUnsupported without changes.
//
// Pending image cleanup that cannot settle within DrainGrace returns
// ErrImageCleanupIncomplete without relinquishing terminal ownership. Upload
// requests remain eligible for later replies. Successful handoff revokes images
// and outstanding clipboard reads. Retransmit
// live images before repainting; accepted deletions remain effective if a later
// step fails. Reacquisition publishes a fresh size because foreground resize
// signals may have belonged to the child.
func (t *Terminal) Hand(prepare, run func() error) (err error) {
	if run == nil {
		return nil
	}
	select {
	case <-t.stop:
		return ErrClosed
	default:
	}
	if !t.waker.interruptible() {
		return fmt.Errorf("term: hand the terminal over: %w", errors.ErrUnsupported)
	}
	if !t.handed.hold() {
		return ErrHandoverActive
	}
	defer t.handed.release()
	if prepare != nil {
		if prepareErr := prepare(); prepareErr != nil {
			return prepareErr
		}
	}
	select {
	case <-t.stop:
		return ErrClosed
	default:
	}
	if releaseErr := t.release(); releaseErr != nil {
		return releaseErr
	}
	// Resume in a defer so a panicking child cannot strand the caller in cooked mode
	// with its terminal still parked. The panic continues after ownership is restored.
	defer func() { err = errors.Join(err, t.resume()) }()
	return run()
}

func (t *Terminal) release() error {
	// Keepalives are output too. Pause before taking the writer's watermark so a
	// refresh cannot appear after the drain and inside the child's output.
	t.task.pause()
	if err := t.writer.pauseAdmission(); err != nil {
		return err
	}
	// Drain before changing modes: a late frame would otherwise enter the
	// child's screen. Refusal here only needs to release the reader claim
	// and resume keepalives, leaving mode ownership intact.
	if err := t.writer.Drain(DrainGrace); err != nil {
		return errors.Join(fmt.Errorf("term: drain before handover: %w", err), t.resumeOutput())
	}
	if err := t.writer.Err(); err != nil {
		return errors.Join(fmt.Errorf("term: drain before handover: %w", err), t.resumeOutput())
	}
	if err := t.awaitUploads(); err != nil {
		return errors.Join(err, t.resumeOutput())
	}

	if err := t.park(t.handed.pause()); err != nil {
		return errors.Join(err, t.resumeOutput())
	}
	t.modeMu.Lock()
	defer t.modeMu.Unlock()
	select {
	case <-t.stop:
		return ErrClosed
	default:
	}
	// Close can seal admission while replies settle. Its seal and this final
	// handoff check share modeMu, so handoff cannot commit after that seal.
	if err := t.writer.pauseAdmission(); err != nil {
		return err
	}
	if err := t.output.SetWriteDeadline(time.Now().Add(DrainGrace)); err != nil {
		return errors.Join(err, t.resumeOutput())
	}
	if err := errors.Join(append(t.giveBack(), t.output.active(false))...); err != nil {
		// release is transactional: on failure no child runs and the session is made
		// live again before the error reaches the caller.
		return errors.Join(err, t.retakeModes())
	}
	t.ownsModes = false
	t.clipboard.Interrupt()
	t.restartInput.Store(true)
	return nil
}

func (t *Terminal) resume() error {
	t.modeMu.Lock()
	defer t.modeMu.Unlock()
	select {
	case <-t.stop:
		return ErrClosed
	default:
	}
	return t.retakeModes()
}

// The caller holds modeMu. From the first attempt, Close again owns cleanup
// even if reacquisition only succeeds in part.
func (t *Terminal) retakeModes() error {
	t.ownsModes = true
	var errs []error
	errs = append(errs, t.output.active(true))
	errs = append(errs, t.output.SetWriteDeadline(time.Now().Add(DrainGrace)))
	if _, err := xterm.MakeRaw(t.inFD); err != nil {
		errs = append(errs, fmt.Errorf("term: enter raw mode: %w", err))
	}
	if err := t.control(t.modes.Enter() + t.title.enter()); err != nil {
		errs = append(errs, fmt.Errorf("term: take the terminal back: %w", err))
	}
	errs = append(errs, t.output.SetWriteDeadline(time.Time{}))
	if err := errors.Join(errs...); err != nil {
		return err
	}
	errs = append(errs, t.resumeOutput())

	// Publish through the watcher's same mailbox, owned by the pump. Even
	// unchanged dimensions require repaint after a child has owned the display.
	t.noteResize(t.Size, true)
	return errors.Join(errs...)
}

func (t *Terminal) resumeOutput() error {
	t.writer.resumeAdmission()
	return t.task.restore(t.writer.Queue)
}

func (t *Terminal) park(parked <-chan struct{}) error {
	t.waker.wake()

	grace := time.NewTimer(parkGrace)
	defer grace.Stop()
	select {
	case <-parked:
		return nil
	case <-t.readerDone:
		return nil
	case <-grace.C:
		return fmt.Errorf("term: wait for reader before handover: %w", os.ErrDeadlineExceeded)
	case <-t.stop:
		return ErrClosed
	}
}

// SetProgress changes task progress outside the cell grid. Unsupported terminals
// ignore it. Repeating an unchanged value writes nothing; active values are refreshed
// often enough for terminals that expire the indicator.
func (t *Terminal) SetProgress(progress Progress) error {
	return t.task.to(progress, t.writer.Queue)
}

// Measure under the same lock as publication: a delayed watcher must not
// overwrite a newer handover measurement. Handover forces a repaint even
// when the child returned the same size, because the display may have changed.
func (t *Terminal) noteResize(measure func() (int, int, error), force bool) {
	t.resizeMu.Lock()
	defer t.resizeMu.Unlock()
	width, height, err := measure()
	if err == nil && (t.size.observe(width, height) || force) {
		t.publishResize(width, height)
	}
}

// publishResize offers the newest measured size to the input pump. Dimensions are
// replaceable state: when the pump has not consumed an older observation, replace it
// instead of dropping the newer truth or blocking the platform watcher.
//
// The caller holds resizeMu, which is also what serializes the producers below.
func (t *Terminal) publishResize(width, height int) {
	latest := input.Resize{Width: width, Height: height}
	select {
	case t.resized <- latest:
		return
	default:
	}
	select {
	case <-t.resized:
	default:
	}
	// Producers are serialized by resizeMu. After removing their older value the
	// mailbox has room, even if the pump raced and consumed that value first.
	t.resized <- latest
}
