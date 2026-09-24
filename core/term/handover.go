package term

import (
	"errors"
	"fmt"
	"sync"
	"time"

	xterm "golang.org/x/term"
)

// parkGrace is how long a handover waits for the reader to come off the terminal.
//
// It normally takes no time at all: the reader waits for the terminal to have
// something to say before it reads, so waking it is enough. The wait is for the
// one case that is not instant — the reader holding a chunk nothing downstream is
// taking, because the interface's own goroutine is inside this call and is not
// draining events. Going ahead anyway is the right answer there: the cost is one
// chunk of input read by this process instead of by the child, and the cost of
// waiting for ever is a program that cannot be got out of.
const parkGrace = 100 * time.Millisecond

// handover is the reader's side of giving the terminal up: a latch it parks on,
// and a signal saying it has.
//
// It is a latch rather than a flag because parking has to be a wait. A flag would
// be read between two reads and leave the reader inside the next one, which is
// exactly where it must not be: a byte read there is a byte the child never sees.
type handover struct {
	mu sync.Mutex
	// resume is closed to let the reader go on. Nil when the terminal is ours.
	resume chan struct{}
	// parked is closed by the reader once it has stopped reading, and nil once that
	// has happened, so it cannot be closed twice.
	parked chan struct{}
}

// hold takes the terminal away from the reader and returns the channel that closes
// once the reader has noticed.
func (h *handover) hold() <-chan struct{} {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.resume = make(chan struct{})
	h.parked = make(chan struct{})
	return h.parked
}

// release gives it back. It is safe to call when nothing was ever held, which is
// what makes a failed handover recoverable by simply resuming.
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
	if resume != nil {
		// Answered once: the signal is what a handover waits on, and closing it twice
		// would be a panic on the second read of the terminal.
		h.parked = nil
	}
	h.mu.Unlock()

	if resume == nil {
		return
	}
	if parked != nil {
		close(parked)
	}
	select {
	case <-resume:
	case <-stop:
	}
}

// Hand gives the terminal to something else and takes it back when it returns.
//
// It is what opening an editor or a pager is made of. The session is put back exactly
// as it was found — the modes it turned on, off in the opposite order, then cooked
// mode — and then the whole of that is done again in reverse.
//
// The reader comes off the terminal first and goes back on last, which is the part
// nothing else can do for a caller: a session that only restored the modes would
// still be reading, and every second keystroke would go to this process.
//
// It runs on the caller's goroutine and does not return until run does, because an
// interface that drew a frame while a child owned the terminal would draw it over the
// child. The caller is responsible for nothing else writing meanwhile.
//
// The window may be a different size afterwards with nothing having reported it,
// because the signal went to whichever process group was in the foreground. A fresh
// size is asked for and delivered on [Terminal.Events].
//
// Where the reader cannot be taken off the terminal this reports
// [errors.ErrUnsupported] and does nothing, because handing over while still reading
// is a child that drops every other keystroke. Whether it can is a question about the
// session rather than the platform: a console can be waited on, and a pipe pretending
// to be one cannot.
func (t *Terminal) Hand(run func() error) (err error) {
	if run == nil {
		return nil
	}
	if !t.waker.interruptible() {
		return fmt.Errorf("term: hand the terminal over: %w", errors.ErrUnsupported)
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
	// Whatever the interface drew has to reach the terminal before the modes go
	// back, for the same reason it does on the way out: a frame written after the
	// alternate screen was given up is a frame drawn onto the user's own screen. Do
	// this before changing any state, so a timeout leaves ownership exactly where it
	// was and needs no compensating transition.
	if err := t.writer.Drain(DrainGrace); err != nil {
		t.task.restore(t.writer.Queue)
		return fmt.Errorf("term: drain before handover: %w", err)
	}
	if err := t.writer.Err(); err != nil {
		t.task.restore(t.writer.Queue)
		return fmt.Errorf("term: drain before handover: %w", err)
	}

	// The reader comes off only after output has settled. From here on, what the
	// terminal says belongs to whoever it is being handed to.
	t.park()
	if err := t.output.SetWriteDeadline(time.Now().Add(DrainGrace)); err != nil {
		return errors.Join(err, t.resume())
	}
	if err := errors.Join(append(t.giveBack(), t.output.active(false))...); err != nil {
		// release is transactional: on failure no child runs and the session is made
		// live again before the error reaches the caller.
		return errors.Join(err, t.resume())
	}
	return nil
}

func (t *Terminal) resume() error {
	var errs []error
	errs = append(errs, t.output.active(true))
	errs = append(errs, t.output.SetWriteDeadline(time.Now().Add(DrainGrace)))
	if _, err := xterm.MakeRaw(t.inFD); err != nil {
		errs = append(errs, fmt.Errorf("term: enter raw mode: %w", err))
	}
	if _, err := t.output.WriteString(t.modes.Enter() + t.title.enter()); err != nil {
		errs = append(errs, fmt.Errorf("term: take the terminal back: %w", err))
	}
	errs = append(errs, t.output.SetWriteDeadline(time.Time{}))
	t.task.restore(t.writer.Queue)
	t.handed.release()

	// The same latest-value mailbox a window resize uses, rather than the public
	// event queue: the pump owns and closes that queue. Report even an unchanged size
	// because foreground signals belonged to the child while it held the terminal,
	// and the program must rebuild the screen whose contents the child replaced. The
	// measurement becomes the watcher's too — those same signals are the ones it did
	// not get, so what it remembers may be a size that has not been true for a while.
	if width, height, err := t.Size(); err == nil {
		t.retakeResize(width, height)
	}
	return errors.Join(errs...)
}

func (t *Terminal) park() {
	parked := t.handed.hold()
	t.waker.wake()

	grace := time.NewTimer(parkGrace)
	defer grace.Stop()
	select {
	case <-parked:
	case <-grace.C:
	case <-t.stop:
	}
}
