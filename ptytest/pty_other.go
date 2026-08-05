//go:build !darwin && !linux

package ptytest

import (
	"os"
	"os/exec"
)

// The assertions in this package are byte-stream arithmetic and work anywhere. It
// is only allocating a pty that does not, so the package still builds and only
// [Start] refuses.

func openPTY() (primary, replica *os.File, err error) { return nil, nil, ErrUnsupported }

func attach(*exec.Cmd, *os.File) {}

func setSize(*os.File, Size) error { return ErrUnsupported }

func signalResize(*os.Process) error { return ErrUnsupported }

func readClosed(error) bool { return false }
