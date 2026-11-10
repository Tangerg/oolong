//go:build darwin

package process

import (
	"os"
	"unsafe"

	"golang.org/x/sys/unix"
)

const supported = true

// Darwin's wait4 ignores WNOWAIT. Its waitid siginfo ABI is needed because
// x/sys does not provide the libSystem waitid wrapper or siginfo type.
type childSiginfo struct {
	signo, errno, code int32
	pid                int32
	uid                uint32
	status             int32
	address            uintptr
	value              uintptr
	band               int64
	reserved           [7]uint64
}

func observeExit(pid int) (int, os.Signal, error) {
	// waitid's idtype selecting one process by its ID; Darwin's P_PID is 1.
	const idTypePID = 1
	var info childSiginfo
	//nolint:staticcheck,gosec // SA1019/G103: x/sys has no waitid wrapper; this exact Darwin ABI buffer stays live for the synchronous syscall.
	_, _, errno := unix.Syscall6(unix.SYS_WAITID, idTypePID, uintptr(pid), uintptr(unsafe.Pointer(&info)), unix.WEXITED|unix.WNOWAIT, 0, 0)
	if errno != 0 {
		return 0, nil, errno
	}
	return childOutcome(info.code, info.status)
}
