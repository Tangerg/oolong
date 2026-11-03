//go:build linux

package process

import (
	"os"
	"unsafe"

	"golang.org/x/sys/unix"
)

const supported = true

func observeExit(pid int) (int, os.Signal, error) {
	var info unix.Siginfo
	if err := unix.Waitid(unix.P_PID, pid, &info, unix.WEXITED|unix.WNOWAIT, nil); err != nil {
		return 0, nil, err
	}
	// Linux aligns siginfo's union to a native word after three C ints. x/sys
	// exposes the common header, but keeps the SIGCHLD payload opaque.
	const headerWords = 3
	const wordBytes = unsafe.Sizeof(uintptr(0))
	const headerBytes = headerWords * unsafe.Sizeof(info.Signo)
	const payloadOffset = (headerBytes + wordBytes - 1) &^ (wordBytes - 1)
	type childStatus struct {
		_      int32
		_      uint32
		status int32
	}
	status := (*childStatus)(unsafe.Add(unsafe.Pointer(&info), payloadOffset)).status //nolint:gosec // G103: the SIGCHLD union is contained in the live, kernel-filled unix.Siginfo.
	return childOutcome(info.Code, status)
}
