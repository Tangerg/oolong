//go:build !unix && !windows

package term

// waker is what there is where a reader cannot be interrupted: nothing.
//
// Two platforms can be woken and this is everywhere else — a build for something
// with neither a pipe to poll nor a console to wait on.
//
// It is a type rather than an absence so that the reader is written once. The read
// below it does the waiting, which is what a reader that cannot be woken does
// anyway.
type waker struct{}

func newWaker(int) (*waker, error) { return &waker{}, nil }

// interruptible reports that the reader cannot be taken off the terminal here.
// Everything that depends on being able to — handing the terminal to a child,
// suspending — asks this and refuses rather than pretending.
func (w *waker) interruptible() bool { return false }

// wait reports that the terminal may have something to say, always. There is
// nothing to ask, so the answer is the one that costs nothing to be wrong about:
// the read that follows blocks until there is.
func (w *waker) wait() (bool, error) { return true, nil }

func (w *waker) wake() {}

func (w *waker) close() {}
