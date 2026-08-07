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
// It is what opening an editor, a pager, or anything else that wants the terminal
// for itself is made of. The session is put back exactly as it was found — the
// modes it turned on, off in the opposite order, then cooked mode — the child runs
// with a terminal that has no idea a program was using it, and then the whole of
// that is done again in reverse.
//
// The reader comes off the terminal first and goes back on last. That is the part
// nothing else can do for a caller: a session that only restored the modes would
// still be reading, and every second keystroke would go to this process instead of
// to the child.
//
// It runs on the caller's goroutine and does not return until run does, which is
// the point — an interface that drew a frame while a child owned the terminal would
// draw it over the child. The caller is responsible for there being nothing else
// writing meanwhile, usually by calling this from its single owner goroutine.
//
// The window may be a different size afterwards, and nothing will have reported it:
// the signal went to whichever process group was in the foreground. A fresh size is
// asked for and delivered on [Terminal.Events], the same way a resize is.
//
// Where the reader cannot be taken off the terminal this reports
// [errors.ErrUnsupported] and does nothing. Handing over while still reading is not
// a lesser version of this; it is a child that drops every other keystroke. Whether
// it can is a question about the session and not about the platform: a console can
// be waited on, and a pipe pretending to be one cannot.
func (t *Terminal) Hand(run func() error) error {
	if run == nil {
		return nil
	}
	if !t.waker.interruptible() {
		return fmt.Errorf("term: hand the terminal over: %w", errors.ErrUnsupported)
	}
	if err := t.release(); err != nil {
		// Nothing ran, so nothing of the child's is lost by taking it straight back —
		// and a terminal left half given away is worse than either state.
		return errors.Join(err, t.resume())
	}
	return errors.Join(run(), t.resume())
}

// Suspend hands the terminal back and stops this process, the way Ctrl+Z does in a
// shell, returning when the process is continued.
//
// It is [Terminal.Hand] with [Suspend] inside it, and it is only separate because
// the terminal has to be given back before the process stops rather than after: a
// program stopped while holding the alternate screen leaves the user looking at
// half an interface with no way to type into it.
func (t *Terminal) Suspend() error { return t.Hand(Suspend) }

// release gives the terminal up without ending the session.
func (t *Terminal) release() error {
	// The reader first, and everything else after: from here on, what the terminal
	// says belongs to whoever it was handed to.
	t.park()

	// Whatever the interface drew has to reach the terminal before the modes go
	// back, for the same reason it does on the way out: a frame written after the
	// alternate screen was given up is a frame drawn onto the user's own screen.
	t.writer.Drain(DrainGrace)
	return errors.Join(t.giveBack()...)
}

// resume takes the terminal back.
func (t *Terminal) resume() error {
	var errs []error
	if _, err := xterm.MakeRaw(int(t.in.Fd())); err != nil {
		errs = append(errs, fmt.Errorf("term: enter raw mode: %w", err))
	}
	if _, err := t.out.WriteString(t.modes.enter() + t.title.enter()); err != nil {
		errs = append(errs, fmt.Errorf("term: take the terminal back: %w", err))
	}
	t.handed.release()

	// The same latest-value mailbox a window resize uses, rather than the public
	// event queue: the pump owns and closes that queue. Report even an unchanged size
	// because foreground signals belonged to the child while it held the terminal,
	// and the program must rebuild the screen whose contents the child replaced.
	if width, height, err := t.Size(); err == nil {
		t.reportResize(width, height)
	}
	return errors.Join(errs...)
}

// park takes the reader off the terminal and waits for it to say it is off.
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
