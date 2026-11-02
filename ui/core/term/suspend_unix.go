//go:build unix

package term

import (
	"errors"
	"fmt"
	"runtime"
)

var errSuspendSignalBlocked = fmt.Errorf("SIGTSTP is blocked on the calling thread: %w", errors.ErrUnsupported)

// Suspend borrows the calling thread to deliver SIGTSTP before returning, so
// [Terminal.Hand] cannot reclaim the terminal ahead of the stop. The operating
// system owns stopping and continuation; an ignored signal, an orphaned process
// group, or a host-installed handler may leave the process running.
//
// Suspend preserves the host's signal policy. A platform that cannot deliver the
// signal synchronously, or a caller that blocks it, reports [errors.ErrUnsupported].
func Suspend() error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	if err := suspendProcess(); err != nil {
		return fmt.Errorf("term: suspend: %w", err)
	}
	return nil
}
