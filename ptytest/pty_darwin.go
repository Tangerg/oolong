//go:build darwin

package ptytest

import (
	"bytes"
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

	ioctl := func(request uintptr, arg uintptr) error {
		if _, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(fd), request, arg); errno != 0 {
			return errno
		}
		return nil
	}
	if err := ioctl(unix.TIOCPTYGRANT, 0); err != nil {
		return nil, nil, fmt.Errorf("ptytest: grant the pty: %w", err)
	}
	if err := ioctl(unix.TIOCPTYUNLK, 0); err != nil {
		return nil, nil, fmt.Errorf("ptytest: unlock the pty: %w", err)
	}
	var name [128]byte
	if err := ioctl(unix.TIOCPTYGNAME, uintptr(unsafe.Pointer(&name[0]))); err != nil {
		return nil, nil, fmt.Errorf("ptytest: name the replica: %w", err)
	}
	end := bytes.IndexByte(name[:], 0)
	if end <= 0 {
		return nil, nil, fmt.Errorf("ptytest: the replica has no name")
	}

	path := string(name[:end])
	replicaFD, err := unix.Open(path, unix.O_RDWR|unix.O_NOCTTY|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, nil, fmt.Errorf("ptytest: open the replica %q: %w", path, err)
	}
	keep = true
	return os.NewFile(uintptr(fd), "/dev/ptmx"), os.NewFile(uintptr(replicaFD), path), nil
}
