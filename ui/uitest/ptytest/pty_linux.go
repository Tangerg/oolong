//go:build linux

package ptytest

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

// openPTY allocates a pty pair the way Linux wants: unlock, then build the
// replica's path from the index it reports.
func openPTY() (primary, replica *os.File, err error) {
	// The primary is opened non-blocking so that os.NewFile registers it with the
	// runtime's poller. That is what makes a read on it interruptible: Close can
	// then end a read that the far end is never going to end, instead of waiting
	// for one. The replica stays blocking — it becomes the child's terminal, and a
	// non-blocking terminal is not one any program expects.
	fd, err := unix.Open("/dev/ptmx", unix.O_RDWR|unix.O_CLOEXEC|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, nil, fmt.Errorf("ptytest: open /dev/ptmx: %w", err)
	}
	keep := false
	defer func() {
		if !keep {
			_ = unix.Close(fd)
		}
	}()

	if unlockErr := unix.IoctlSetPointerInt(fd, unix.TIOCSPTLCK, 0); unlockErr != nil {
		return nil, nil, fmt.Errorf("ptytest: unlock the pty: %w", unlockErr)
	}
	index, indexErr := unix.IoctlGetInt(fd, unix.TIOCGPTN)
	if indexErr != nil {
		return nil, nil, fmt.Errorf("ptytest: number the replica: %w", indexErr)
	}

	path := fmt.Sprintf("/dev/pts/%d", index)
	replicaFD, openErr := unix.Open(path, unix.O_RDWR|unix.O_NOCTTY|unix.O_CLOEXEC, 0)
	if openErr != nil {
		return nil, nil, fmt.Errorf("ptytest: open the replica %q: %w", path, openErr)
	}
	keep = true
	return os.NewFile(uintptr(fd), "/dev/ptmx"), os.NewFile(uintptr(replicaFD), path), nil
}
