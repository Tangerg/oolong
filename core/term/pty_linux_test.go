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
