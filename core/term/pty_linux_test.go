//go:build linux

package term_test

import (
	"fmt"

	"golang.org/x/sys/unix"
)

// ptyName asks a pty primary for its replica's path.
func ptyName(fd int) (string, error) {
	if err := unix.IoctlSetPointerInt(fd, unix.TIOCSPTLCK, 0); err != nil {
		return "", err
	}
	index, err := unix.IoctlGetInt(fd, unix.TIOCGPTN)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("/dev/pts/%d", index), nil
}
