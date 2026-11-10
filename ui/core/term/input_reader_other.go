//go:build !unix && !windows

package term

import (
	"errors"
	"os"
	"time"
)

// Source portability does not imply an owned terminal input adapter. Refuse
// unsupported input instead of starting a read that shutdown cannot join.
type inputReader struct{}

func newInputReader(int) (*inputReader, error) { return &inputReader{}, nil }

func (w *inputReader) interruptible() bool { return false }

func (w *inputReader) read(*os.File) readResult {
	return readResult{at: time.Now(), err: errors.ErrUnsupported}
}

func (w *inputReader) reset() {}

func (w *inputReader) buffered() bool { return false }

func (w *inputReader) wake() {}

func (w *inputReader) close() {}
