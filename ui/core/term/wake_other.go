//go:build !unix && !windows

package term

import (
	"errors"
	"os"
)

// Source portability does not imply an owned terminal input adapter. Refuse
// unsupported input instead of starting a read that shutdown cannot join.
type waker struct{}

func newWaker(int) (*waker, error) { return &waker{}, nil }

// interruptible reports that the reader cannot be taken off the terminal here.
// Everything that depends on being able to — handing the terminal to a child,
// suspending — asks this and refuses rather than pretending.
func (w *waker) interruptible() bool { return false }

func (w *waker) read(*os.File) ([]byte, error) { return nil, errors.ErrUnsupported }

func (w *waker) reset() {}

func (w *waker) buffered() bool { return false }

func (w *waker) wake() {}

func (w *waker) close() {}
