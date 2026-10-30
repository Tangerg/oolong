//go:build unix

package term

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

// The owned duplicate is registered with Go's poller while nonblocking. Never call
// File.Fd on it: that switches it back to blocking and defeats write deadlines.
// Descriptor flags belong to the shared open-file description, so handover and
// shutdown restore the caller's original mode only after our writer has stopped.
type terminalOutput struct {
	*os.File
	original *os.File
	flags    int
}

func newTerminalOutput(original *os.File) (*terminalOutput, error) {
	raw, err := original.SyscallConn()
	if err != nil {
		return nil, err
	}
	fd, flags := -1, 0
	var setup error
	err = raw.Control(func(source uintptr) {
		flags, setup = unix.FcntlInt(source, unix.F_GETFL, 0)
		if setup != nil {
			return
		}
		fd, setup = unix.Dup(int(source))
		if setup != nil {
			return
		}
		unix.CloseOnExec(fd)
		setup = unix.SetNonblock(fd, true)
	})
	if err != nil || setup != nil {
		if fd >= 0 {
			_ = unix.Close(fd)
		}
		return nil, errors.Join(err, setup)
	}
	return &terminalOutput{File: os.NewFile(uintptr(fd), original.Name()), original: original, flags: flags}, nil
}

func (o *terminalOutput) active(active bool) error {
	raw, err := o.original.SyscallConn()
	if err != nil {
		return err
	}
	flags := o.flags
	if active {
		flags |= unix.O_NONBLOCK
	}
	var result error
	err = raw.Control(func(fd uintptr) { _, result = unix.FcntlInt(fd, unix.F_SETFL, flags) })
	return errors.Join(err, result)
}

func (o *terminalOutput) Close() error {
	return errors.Join(o.File.Close(), o.active(false))
}
