package process

import (
	"fmt"
	"os"
	"syscall"
)

// SignalGroup requires a child group leader whose PID remains reserved until
// return. The caller owns final reaping and must finish all group signals first.
// An absent group or a confirmed zombie-only group needs no further signal.
func SignalGroup(leader *os.Process, signal syscall.Signal) error {
	if leader == nil || leader.Pid <= 1 {
		return fmt.Errorf("process: signal child group: %w", os.ErrInvalid)
	}
	err := signalGroup(leader.Pid, signal)
	//nolint:errorlint // Only a bare Kill error proves absence; inspection errors may be joined with EPERM.
	if err == nil || err == syscall.ESRCH {
		return nil
	}
	return fmt.Errorf("process: signal child group: %w", err)
}
