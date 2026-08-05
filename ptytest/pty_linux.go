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
	fd, err := unix.Open("/dev/ptmx", unix.O_RDWR|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, nil, fmt.Errorf("ptytest: open /dev/ptmx: %w", err)
	}
	keep := false
	defer func() {
		if !keep {
			_ = unix.Close(fd)
		}
	}()

	if err := unix.IoctlSetPointerInt(fd, unix.TIOCSPTLCK, 0); err != nil {
		return nil, nil, fmt.Errorf("ptytest: unlock the pty: %w", err)
	}
	index, err := unix.IoctlGetInt(fd, unix.TIOCGPTN)
	if err != nil {
		return nil, nil, fmt.Errorf("ptytest: number the replica: %w", err)
	}

	path := fmt.Sprintf("/dev/pts/%d", index)
	replicaFD, err := unix.Open(path, unix.O_RDWR|unix.O_NOCTTY|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, nil, fmt.Errorf("ptytest: open the replica %q: %w", path, err)
	}
	keep = true
	return os.NewFile(uintptr(fd), "/dev/ptmx"), os.NewFile(uintptr(replicaFD), path), nil
}
