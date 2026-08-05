//go:build !unix

package term

// interruptible says whether a reader here can be taken off the terminal without
// waiting for a keystroke. It cannot: there is no portable way to end a blocking
// read on a terminal, and everything that depends on being able to — handing the
// terminal to a child, suspending — asks this and refuses rather than pretending.
const interruptible = false

// waker is what there is where a reader cannot be interrupted: nothing.
//
// It is a type rather than an absence so that the reader is written once. The read
// below it does the waiting, which is what a reader that cannot be woken does
// anyway.
type waker struct{}

func newWaker(int) (*waker, error) { return &waker{}, nil }

// wait reports that the terminal may have something to say, always. There is
// nothing to ask, so the answer is the one that costs nothing to be wrong about:
// the read that follows blocks until there is.
func (w *waker) wait() (bool, error) { return true, nil }

func (w *waker) wake() {}

func (w *waker) close() {}
