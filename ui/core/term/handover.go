package term

import (
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	xterm "golang.org/x/term"
)

// ErrHandoverActive means another Hand already owns terminal reacquisition.
var ErrHandoverActive = errors.New("term: terminal handover already active")

// An input queue waiting on the UI owner cannot drain while that owner is
// handing over the terminal. Refuse the handover before changing modes in
// that case: only the reader can confirm it stopped consuming keystrokes.
const parkGrace = 100 * time.Millisecond

// handover parks the sole reader before input ownership changes. A flag
// checked between reads would leave a race in which the next read steals the
// child's keystroke.
type handover struct {
	mu sync.Mutex
	// resume is closed to let the reader go on. Nil when no handover is claimed.
	resume chan struct{}
	// Parking is requested only after terminal replies have settled. The sole
	// reader closes parked once before blocking until this claim is released.
	parked chan struct{}
}

// hold claims the handover lifetime before any modes or reader state can change.
// Input remains available for outstanding replies until pause requests parking.
func (h *handover) hold() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.resume != nil {
		return false
	}
	h.resume = make(chan struct{})
	return true
}

func (h *handover) pause() <-chan struct{} {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.parked = make(chan struct{})
	return h.parked
}

// release ends the claim after restoration or rollback, including a child panic.
func (h *handover) release() {
	h.mu.Lock()
	resume := h.resume
	h.resume, h.parked = nil, nil
	h.mu.Unlock()
	if resume != nil {
		close(resume)
	}
}

// park is what the reader calls before each read. It returns at once unless the
// terminal has been handed over, and otherwise waits until it comes back — or
// until the session ends, because a terminal handed over and never taken back must
// not keep a goroutine alive for ever.
func (h *handover) park(stop <-chan struct{}) {
	h.mu.Lock()
	resume, parked := h.resume, h.parked
	h.mu.Unlock()

	if parked == nil {
		return
	}
	close(parked)
	select {
	case <-resume:
	case <-stop:
	}
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
	if t.stopped() {
		return ErrClosed
	}
	if !t.inputReader.interruptible() {
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
	if t.stopped() {
		return ErrClosed
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
	if t.stopped() {
		return ErrClosed
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
	if t.stopped() {
		return ErrClosed
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
		// The display is in a state nobody can describe. Sealing admission makes
		// every later frame report this failure instead of a handover that is
		// no longer happening.
		t.writer.seal(fmt.Errorf("term: take the terminal back: %w", err))
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
	t.inputReader.wake()

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
