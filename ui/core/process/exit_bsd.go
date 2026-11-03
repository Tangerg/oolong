//go:build dragonfly || freebsd || netbsd

package process

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

const supported = true

func observeExit(pid int) (int, os.Signal, error) {
	var status unix.WaitStatus
	if _, err := unix.Wait4(pid, &status, unix.WNOWAIT, nil); err != nil {
		return 0, nil, err
	}
	if status.Signaled() {
		return -1, status.Signal(), nil
	}
	if !status.Exited() {
		return 0, nil, fmt.Errorf("process: child is not exited: status %d", status)
	}
	return status.ExitStatus(), nil, nil
}
