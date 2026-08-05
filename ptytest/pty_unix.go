//go:build darwin || linux

package ptytest

import (
	"errors"
	"os"
	"os/exec"
	"syscall"

	"golang.org/x/sys/unix"
)

// attach makes the replica the child's controlling terminal, which is what makes
// the child believe it is talking to one.
func attach(cmd *exec.Cmd, replica *os.File) {
	cmd.Stdin = replica
	cmd.Stdout = replica
	cmd.Stderr = replica
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid:  true,
		Setctty: true,
		Ctty:    0,
	}
}

func setSize(primary *os.File, size Size) error {
	return unix.IoctlSetWinsize(int(primary.Fd()), unix.TIOCSWINSZ, &unix.Winsize{
		Col: uint16(size.Cols),
		Row: uint16(size.Rows),
	})
}

func signalResize(process *os.Process) error {
	if process == nil {
		return nil
	}
	return process.Signal(syscall.SIGWINCH)
}

// readClosed reports whether an error is just the far end of the pty going away.
//
// Linux reports EIO when the last replica closes, which is the ordinary end of a
// session rather than a failure. Darwin usually reports EOF, and treating EIO as
// the same thing there costs nothing.
func readClosed(err error) bool { return errors.Is(err, syscall.EIO) }
