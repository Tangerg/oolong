package term_test

import (
	"testing"
	"unsafe"

	"golang.org/x/sys/unix"
)

func blockSuspendSignal(t *testing.T) func() {
	t.Helper()
	const (
		blockMask   = 1
		replaceMask = 3
	)
	var mask, previous unix.Sigset_t
	mask.Val[0] = 1 << (unix.SIGTSTP - 1)
	//nolint:gosec // G103: x/sys has no signal-mask wrapper; both platform sigset_t values remain live through this synchronous fixture syscall.
	if _, _, errno := unix.RawSyscall(unix.SYS_SIGPROCMASK, blockMask,
		uintptr(unsafe.Pointer(&mask)), uintptr(unsafe.Pointer(&previous))); errno != 0 {
		t.Fatal(errno)
	}
	return func() {
		//nolint:gosec // G103: restore the captured platform sigset_t before the fixture releases its locked thread.
		if _, _, errno := unix.RawSyscall(unix.SYS_SIGPROCMASK, replaceMask,
			uintptr(unsafe.Pointer(&previous)), 0); errno != 0 {
			t.Errorf("restore thread signal mask: %v", errno)
		}
	}
}
