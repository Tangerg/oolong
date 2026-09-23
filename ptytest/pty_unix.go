//go:build darwin || linux

package ptytest

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"syscall"

	"golang.org/x/sys/unix"
)

// supported says this platform has a pty.
const supported = true

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

// setSize tells the pty how big it is.
//
// The descriptor is borrowed through the syscall connection rather than taken with
// Fd: taking it puts the file back into blocking mode and out of the poller, which
// is the one property Close depends on to end a read nobody else will.
func setSize(primary *os.File, size Size) error {
	cols, rows, err := size.dims()
	if err != nil {
		return err
	}
	conn, err := primary.SyscallConn()
	if err != nil {
		return fmt.Errorf("ptytest: size the pty: %w", err)
	}
	var ioctlErr error
	if controlErr := conn.Control(func(fd uintptr) {
		ioctlErr = unix.IoctlSetWinsize(int(fd), unix.TIOCSWINSZ, &unix.Winsize{
			Col: cols,
			Row: rows,
		})
	}); controlErr != nil {
		return fmt.Errorf("ptytest: size the pty: %w", controlErr)
	}
	return ioctlErr
}

// endSession kills everything the child left running, not only the child.
//
// attach gives it a session and a process group of its own, so what the harness
// started is a group. Killing the leader alone leaves whatever it spawned holding
// the replica open: the terminal stays alive, the transcript reader has nothing to
// end it, and a test that asked for a pty walks away from a process still sitting
// on one.
func endSession(process *os.Process) error {
	if process == nil {
		return nil
	}
	// A group kill is addressed by negating the leader's id, so a leader that is
	// not one — init, or a zero value — would address something else entirely.
	if process.Pid <= 1 {
		return process.Kill()
	}
	if err := unix.Kill(-process.Pid, unix.SIGKILL); err != nil && !errors.Is(err, unix.ESRCH) {
		return process.Kill()
	}
	return nil
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
