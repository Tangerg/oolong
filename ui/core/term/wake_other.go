//go:build !unix && !windows

package term

import (
	"errors"
	"os"
	"time"
)

// Source portability does not imply an owned terminal input adapter. Refuse
// unsupported input instead of starting a read that shutdown cannot join.
type waker struct{}

func newWaker(int) (*waker, error) { return &waker{}, nil }

func (w *waker) interruptible() bool { return false }

func (w *waker) read(*os.File) readResult {
	return readResult{at: time.Now(), err: errors.ErrUnsupported}
}

func (w *waker) reset() {}

func (w *waker) buffered() bool { return false }

func (w *waker) wake() {}

func (w *waker) close() {}
