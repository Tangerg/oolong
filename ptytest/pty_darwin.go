//go:build darwin

package ptytest

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"unsafe"

	"golang.org/x/sys/unix"
)

// openPTY allocates a pty pair the way macOS wants: grant, unlock, then ask for
// the replica's name rather than working it out from an index.
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

	// Granting and unlocking take no argument, so the package's own wrapper does.
	if grantErr := unix.IoctlSetInt(fd, unix.TIOCPTYGRANT, 0); grantErr != nil {
		return nil, nil, fmt.Errorf("ptytest: grant the pty: %w", grantErr)
	}
	if unlockErr := unix.IoctlSetInt(fd, unix.TIOCPTYUNLK, 0); unlockErr != nil {
		return nil, nil, fmt.Errorf("ptytest: unlock the pty: %w", unlockErr)
	}

	// Asking the replica's name does take one — a buffer — and x/sys/unix has no
	// typed wrapper for this particular ioctl, so there is nothing to call but the
	// syscall. The deprecation asks for a libSystem wrapper that the package does
	// not offer here; the alternative is not using a pty on macOS at all.
	var name [128]byte
	//nolint:staticcheck,gosec // SA1019/G103: no wrapper exists for TIOCPTYGNAME, which needs a buffer.
	if _, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(fd), unix.TIOCPTYGNAME,
		uintptr(unsafe.Pointer(&name[0]))); errno != 0 {
		return nil, nil, fmt.Errorf("ptytest: name the replica: %w", errno)
	}
	end := bytes.IndexByte(name[:], 0)
	if end <= 0 {
		return nil, nil, errors.New("ptytest: the replica has no name")
	}

	path := string(name[:end])
	replicaFD, err := unix.Open(path, unix.O_RDWR|unix.O_NOCTTY|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, nil, fmt.Errorf("ptytest: open the replica %q: %w", path, err)
	}
	keep = true
	return os.NewFile(uintptr(fd), "/dev/ptmx"), os.NewFile(uintptr(replicaFD), path), nil
}
