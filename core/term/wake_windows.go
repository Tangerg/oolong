//go:build windows

package term

import (
	"fmt"
	"sync"

	"golang.org/x/sys/windows"
)

// waker is a wait for the terminal to have something to say that this process can
// end on its own.
//
// The shape is the same as everywhere else and the mechanism is what the platform
// gives: a console is a waitable object, so waiting on it and on an event this
// process can set is one call. What comes back from that wait has read nothing,
// which is the property the whole thing exists for — a byte taken there is a byte
// the child it was handed to never sees.
//
// # What it cannot promise
//
// A console is signalled for anything in its input queue, and not everything in
// there is a keystroke: a mouse report, a focus change and a window resize are
// records too. A wait woken by one of those falls into a read that then blocks until
// a key arrives, which is where a reader that could not be interrupted always was.
// The handover notices — it waits a moment for the reader to park and goes ahead
// without it — so the cost is what it has always been on this platform, and the
// common case, where nothing is pending, now works.
type waker struct {
	// input is the console this session reads, and console says it is one. A session
	// whose input is a pipe cannot be woken, which is what [waker.interruptible]
	// reports and what stops a handover from pretending.
	input   windows.Handle
	console bool
	// signal is set when this process wants the reader back. It is manual-reset, so a
	// wake-up that arrives while nobody is waiting is still there when somebody does.
	signal windows.Handle

	mu     sync.Mutex
	closed bool
}

func newWaker(fd int) (*waker, error) {
	input := windows.Handle(fd)
	var mode uint32
	console := windows.GetConsoleMode(input, &mode) == nil

	event, err := windows.CreateEvent(nil, 1, 0, nil)
	if err != nil {
		return nil, fmt.Errorf("term: an event to wake the reader: %w", err)
	}
	return &waker{input: input, console: console, signal: event}, nil
}

// interruptible reports whether the reader can be taken off the terminal without
// waiting for a keystroke, which here means: is what it is reading a console.
func (w *waker) interruptible() bool { return w.console }

// wait blocks until the terminal has something to say, reporting false when this
// process asked for the reader back instead.
func (w *waker) wait() (bool, error) {
	if !w.console {
		// Nothing to wait on, so the read below does the waiting — which is what a
		// reader that cannot be woken did before any of this existed.
		return true, nil
	}
	which, err := windows.WaitForMultipleObjects([]windows.Handle{w.input, w.signal}, false, windows.INFINITE)
	if err != nil {
		return false, fmt.Errorf("term: watch the terminal: %w", err)
	}
	switch which {
	case windows.WAIT_OBJECT_0:
		return true, nil
	case windows.WAIT_OBJECT_0 + 1:
		// Manual-reset, so it is cleared here rather than by the wait: one wake-up
		// must not answer the next one as well.
		_ = windows.ResetEvent(w.signal)
		return false, nil
	default:
		return false, fmt.Errorf("term: watch the terminal: the wait ended with %d", which)
	}
}

// wake ends whatever wait is in progress, and is safe to call when there is none.
func (w *waker) wake() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return
	}
	_ = windows.SetEvent(w.signal)
}

// close releases the event. It belongs to the reader and is called when the reader
// returns, for the reason it is everywhere else: closing a handle out from under a
// wait is closing a number the operating system is free to hand out again.
func (w *waker) close() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return
	}
	w.closed = true
	_ = windows.CloseHandle(w.signal)
}
