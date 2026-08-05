//go:build darwin

package term_test

import (
	"bytes"
	"errors"
	"os"
	"unsafe"

	"golang.org/x/sys/unix"
)

// openPTY allocates a pty pair for a test to drive a terminal through.
//
// The primary is non-blocking so the runtime's poller owns it, which is what
// makes a read deadline work: a plain descriptor would block a test for ever.
func openPTY() (primary, replica *os.File, err error) {
	fd, err := unix.Open("/dev/ptmx", unix.O_RDWR|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, nil, err
	}
	if err := unix.IoctlSetInt(fd, unix.TIOCPTYGRANT, 0); err != nil {
		_ = unix.Close(fd)
		return nil, nil, err
	}
	if err := unix.IoctlSetInt(fd, unix.TIOCPTYUNLK, 0); err != nil {
		_ = unix.Close(fd)
		return nil, nil, err
	}
	var name [128]byte
	//nolint:staticcheck,gosec // SA1019/G103: no wrapper exists for TIOCPTYGNAME, which needs a buffer.
	if _, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(fd), unix.TIOCPTYGNAME,
		uintptr(unsafe.Pointer(&name[0]))); errno != 0 {
		_ = unix.Close(fd)
		return nil, nil, errno
	}
	end := bytes.IndexByte(name[:], 0)
	if end <= 0 {
		_ = unix.Close(fd)
		return nil, nil, errors.New("the replica has no name")
	}
	path := string(name[:end])
	rfd, err := unix.Open(path, unix.O_RDWR|unix.O_NOCTTY|unix.O_CLOEXEC, 0)
	if err != nil {
		_ = unix.Close(fd)
		return nil, nil, err
	}
	if err := unix.SetNonblock(fd, true); err != nil {
		_ = unix.Close(fd)
		_ = unix.Close(rfd)
		return nil, nil, err
	}
	return os.NewFile(uintptr(fd), "/dev/ptmx"), os.NewFile(uintptr(rfd), path), nil
}
