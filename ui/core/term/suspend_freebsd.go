package term

import (
	"unsafe"

	"golang.org/x/sys/unix"
)

func suspendProcess() error {
	// The nil input mask prevents any change to the host's policy. x/sys
	// supplies FreeBSD's sigset_t but no sigprocmask wrapper.
	var mask unix.Sigset_t
	//nolint:gosec // G103: x/sys's FreeBSD sigset_t storage remains live through this synchronous read-only syscall; no wrapper is available.
	if _, _, errno := unix.RawSyscall(unix.SYS_SIGPROCMASK, 0, 0, uintptr(unsafe.Pointer(&mask))); errno != 0 {
		return errno
	}
	if mask.Val[0]&(1<<(unix.SIGTSTP-1)) != 0 {
		return errSuspendSignalBlocked
	}
	// FreeBSD directs a self-signal to the calling thread when it is unblocked;
	// the default stop therefore runs before this locked thread can return.
	return unix.Kill(unix.Getpid(), unix.SIGTSTP)
}
