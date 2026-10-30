//go:build linux

package term_test

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

// openPTY allocates a pty pair for a test to drive a terminal through. See the
// darwin file for why the primary is non-blocking.
func openPTY() (primary, replica *os.File, err error) {
	fd, err := unix.Open("/dev/ptmx", unix.O_RDWR|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, nil, err
	}
	if unlockErr := unix.IoctlSetPointerInt(fd, unix.TIOCSPTLCK, 0); unlockErr != nil {
		_ = unix.Close(fd)
		return nil, nil, unlockErr
	}
	index, indexErr := unix.IoctlGetInt(fd, unix.TIOCGPTN)
	if indexErr != nil {
		_ = unix.Close(fd)
		return nil, nil, indexErr
	}
	path := fmt.Sprintf("/dev/pts/%d", index)
	rfd, openErr := unix.Open(path, unix.O_RDWR|unix.O_NOCTTY|unix.O_CLOEXEC, 0)
	if openErr != nil {
		_ = unix.Close(fd)
		return nil, nil, openErr
	}
	if err := unix.SetNonblock(fd, true); err != nil {
		_ = unix.Close(fd)
		_ = unix.Close(rfd)
		return nil, nil, err
	}
	return os.NewFile(uintptr(fd), "/dev/ptmx"), os.NewFile(uintptr(rfd), path), nil
}

func resizePTY(replica *os.File, width, height int) error {
	const maxDimension = int(^uint16(0))
	if width < 0 || width > maxDimension || height < 0 || height > maxDimension {
		return fmt.Errorf("pty size %dx%d is outside uint16", width, height)
	}
	size := &unix.Winsize{Col: uint16(width), Row: uint16(height)}
	if err := unix.IoctlSetWinsize(int(replica.Fd()), unix.TIOCSWINSZ, size); err != nil {
		return err
	}
	// The test replica is deliberately not this process's controlling terminal, so
	// the kernel has no foreground process group to notify. Deliver the same signal
	// a real controlling terminal would send after the ioctl.
	return unix.Kill(os.Getpid(), unix.SIGWINCH)
}
