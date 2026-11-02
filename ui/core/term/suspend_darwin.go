package term

import (
	"unsafe"

	"golang.org/x/sys/unix"
)

func suspendProcess() error {
	// Darwin's sigset_t is uint32. A nil input mask makes how irrelevant and
	// leaves the host's policy untouched; x/sys exposes no wrapper for this call.
	var mask uint32
	//nolint:staticcheck,gosec // SA1019/G103: Darwin's uint32 sigset_t storage remains live through this synchronous read-only syscall; x/sys has no wrapper.
	if _, _, errno := unix.RawSyscall(unix.SYS___PTHREAD_SIGMASK, 0, 0, uintptr(unsafe.Pointer(&mask))); errno != 0 {
		return errno
	}
	if mask&(1<<(unix.SIGTSTP-1)) != 0 {
		return errSuspendSignalBlocked
	}
	// XNU applies the default stop with task_suspend_internal inside kill,
	// so the calling task cannot reenter user space before continuation.
	return unix.Kill(unix.Getpid(), unix.SIGTSTP)
}
