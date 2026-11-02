package term

import "golang.org/x/sys/unix"

func suspendProcess() error {
	var mask unix.Sigset_t
	if err := unix.PthreadSigmask(unix.SIG_SETMASK, nil, &mask); err != nil {
		return err
	}
	if mask.Val[0]&(1<<(unix.SIGTSTP-1)) != 0 {
		return errSuspendSignalBlocked
	}
	// Process-directed kill may return before another thread receives the stop,
	// letting Hand restore raw mode too early. Target this locked thread instead.
	return unix.Tgkill(unix.Getpid(), unix.Gettid(), unix.SIGTSTP)
}
