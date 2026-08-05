//go:build darwin

package term_test

import (
	"bytes"
	"errors"
	"unsafe"

	"golang.org/x/sys/unix"
)

// ptyName asks a pty primary for its replica's path.
func ptyName(fd int) (string, error) {
	if err := unix.IoctlSetInt(fd, unix.TIOCPTYGRANT, 0); err != nil {
		return "", err
	}
	if err := unix.IoctlSetInt(fd, unix.TIOCPTYUNLK, 0); err != nil {
		return "", err
	}
	var name [128]byte
	//nolint:staticcheck,gosec // SA1019/G103: no wrapper exists for TIOCPTYGNAME, which needs a buffer.
	if _, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(fd), unix.TIOCPTYGNAME,
		uintptr(unsafe.Pointer(&name[0]))); errno != 0 {
		return "", errno
	}
	end := bytes.IndexByte(name[:], 0)
	if end <= 0 {
		return "", errors.New("the replica has no name")
	}
	return string(name[:end]), nil
}
