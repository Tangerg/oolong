//go:build unix

package term

import (
	"fmt"
	"math"
	"sync"

	"golang.org/x/sys/unix"
)

// interruptible says whether a reader here can be taken off the terminal without
// waiting for a keystroke. Everything that depends on that — handing the terminal
// to a child, suspending, a session that stops reading the moment it is closed —
// asks this rather than the operating system's name.
const interruptible = true

// waker is a wait for the terminal to have something to say that this process can
// end on its own.
//
// A blocking read cannot be cancelled portably, which is the usual answer and the
// wrong shape: the goroutine is then inside a read at exactly the moments a session
// needs it not to be. So the waiting happens before the read instead, on the
// terminal and on a pipe this process writes a byte to when it wants the reader
// back. What is woken this way has read nothing, which is the whole property — a
// byte taken here is a byte the child it was handed to never sees.
type waker struct {
	fd int
	// in and out are the two ends of the pipe. One byte on it means "stop waiting";
	// the byte itself says nothing, so several are as good as one.
	in, out int

	mu     sync.Mutex
	closed bool
}

func newWaker(fd int) (*waker, error) {
	var pipe [2]int
	if err := unix.Pipe(pipe[:]); err != nil {
		return nil, fmt.Errorf("term: a pipe to wake the reader: %w", err)
	}
	for _, fd := range pipe {
		// Not inherited by whatever the terminal is handed to, and never able to
		// block: a wake-up that filled the pipe would hang the caller asking for it.
		unix.CloseOnExec(fd)
		if err := unix.SetNonblock(fd, true); err != nil {
			_ = unix.Close(pipe[0])
			_ = unix.Close(pipe[1])
			return nil, fmt.Errorf("term: a pipe to wake the reader: %w", err)
		}
	}
	return &waker{fd: fd, in: pipe[0], out: pipe[1]}, nil
}

// wait blocks until the terminal has something to say, reporting false when this
// process asked for the reader back instead.
func (w *waker) wait() (bool, error) {
	terminal, ok := pollFd(w.fd)
	if !ok {
		return false, fmt.Errorf("term: watch the terminal: descriptor %d is out of range", w.fd)
	}
	wake, ok := pollFd(w.in)
	if !ok {
		return false, fmt.Errorf("term: watch the terminal: descriptor %d is out of range", w.in)
	}
	fds := []unix.PollFd{terminal, wake}
	for {
		if _, err := unix.Poll(fds, -1); err != nil {
			if err == unix.EINTR {
				// A signal arrived — a resize, most often. Nothing was decided, so ask
				// again rather than reporting an end of input that did not happen.
				continue
			}
			return false, fmt.Errorf("term: watch the terminal: %w", err)
		}
		if fds[1].Revents != 0 {
			w.drain()
			return false, nil
		}
		if fds[0].Revents != 0 {
			// Anything at all, including a hangup: what it was is the read's to find
			// out, and a read is what turns an end of input into the error that says so.
			return true, nil
		}
	}
}

// wake ends whatever wait is in progress, and is safe to call when there is none.
func (w *waker) wake() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return
	}
	_, _ = unix.Write(w.out, []byte{0})
}

// close releases the pipe. It belongs to the reader, and is called when the reader
// returns: closing it from anywhere else could close a descriptor out from under a
// wait, and a descriptor number is reused the moment it is free.
func (w *waker) close() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return
	}
	w.closed = true
	_ = unix.Close(w.in)
	_ = unix.Close(w.out)
}

// drain empties the pipe, so that one wake-up does not answer the next wait as well.
func (w *waker) drain() {
	var buf [16]byte
	for {
		if n, err := unix.Read(w.in, buf[:]); n <= 0 || err != nil {
			return
		}
	}
}

// pollFd is one descriptor to watch, and whether it can be watched at all.
//
// The check is on the number rather than on the conversion: a descriptor is an int
// and the field it goes in is 32 bits, and quietly narrowing one would watch a
// descriptor the caller never named.
func pollFd(fd int) (unix.PollFd, bool) {
	if fd < 0 || fd > math.MaxInt32 {
		return unix.PollFd{}, false
	}
	return unix.PollFd{Fd: int32(fd), Events: unix.POLLIN}, true
}
