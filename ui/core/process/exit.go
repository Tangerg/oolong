// Package process separates observing child exit from releasing its kernel
// identity, so a process-group owner can finish signaling before reaping.
package process

import (
	"errors"
	"fmt"
	"os"
	"syscall"
)

// ExitError is an immutable observation, not a reaping
// receipt; the caller must still wait through the original os.Process or exec.Cmd.
// A signaled child has Code -1.
type ExitError struct {
	Code   int
	Signal os.Signal
}

func (e *ExitError) Error() string {
	if e.Signal != nil {
		return "process: " + e.Signal.String()
	}
	return fmt.Sprintf("process: exit status %d", e.Code)
}

func Supported() bool { return supported }

// WaitExit leaves the child waitable, reserving its PID until the caller reaps it.
// The caller must own that final wait and must not enable automatic SIGCHLD reaping.
// Cancellation belongs to the process owner, which can signal while this blocks.
func WaitExit(child *os.Process) error {
	if child == nil || child.Pid <= 0 {
		return fmt.Errorf("process: observe child exit: %w", os.ErrInvalid)
	}
	for {
		code, signal, err := observeExit(child.Pid)
		if errors.Is(err, syscall.EINTR) {
			continue
		}
		if err != nil {
			return fmt.Errorf("process: observe child exit: %w", err)
		}
		if code == 0 && signal == nil {
			return nil
		}
		return &ExitError{Code: code, Signal: signal}
	}
}
