package term_test

import (
	"testing"

	"golang.org/x/sys/unix"
)

func blockSuspendSignal(t *testing.T) func() {
	t.Helper()
	var mask, previous unix.Sigset_t
	mask.Val[0] = 1 << (unix.SIGTSTP - 1)
	if err := unix.PthreadSigmask(unix.SIG_BLOCK, &mask, &previous); err != nil {
		t.Fatal(err)
	}
	return func() {
		if err := unix.PthreadSigmask(unix.SIG_SETMASK, &previous, nil); err != nil {
			t.Errorf("restore thread signal mask: %v", err)
		}
	}
}
