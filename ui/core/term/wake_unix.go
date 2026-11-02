//go:build unix

package term

import (
	"errors"
	"fmt"
	"math"
	"os"
	"sync"
	"syscall"

	"golang.org/x/sys/unix"
)

// waker waits on input readiness and an owned pipe before reading. A wake can
// therefore return input ownership without consuming a byte meant for the child,
// unlike an uncancellable blocking read.
type waker struct {
	fd int
	// in and out are the two ends of the pipe. One byte on it means "stop waiting";
	// the byte itself says nothing, so several are as good as one.
	in, out int

	mu     sync.Mutex
	closed bool
}

// This session's readiness wait always includes an owned wake pipe, so handing
// input back never depends on the user typing a key.
func (w *waker) interruptible() bool { return true }

func newWaker(fd int) (*waker, error) {
	// Pipe and close-on-exec are separate on some Unix targets.
	syscall.ForkLock.RLock()
	defer syscall.ForkLock.RUnlock()
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

// Raw input has one consumer, so readiness permits a bounded read while the
// session still owns its modes. Close joins that read before restoring them.
func (w *waker) read(in *os.File) ([]byte, error) {
	ready, err := w.wait()
	if err != nil || !ready {
		return nil, err
	}
	buf := make([]byte, readChunkBytes)
	n, err := in.Read(buf)
	return buf[:n], err
}

func (w *waker) reset() {}

func (w *waker) buffered() bool { return false }

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
			if errors.Is(err, unix.EINTR) {
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

// Validate descriptor width before narrowing to poll's int32 field; wrapping
// would wait on a descriptor the caller never supplied.
func pollFd(fd int) (unix.PollFd, bool) {
	if fd < 0 || fd > math.MaxInt32 {
		return unix.PollFd{}, false
	}
	return unix.PollFd{Fd: int32(fd), Events: unix.POLLIN}, true
}
