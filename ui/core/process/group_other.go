//go:build !darwin && !linux && !dragonfly && !freebsd && !netbsd

package process

import (
	"errors"
	"syscall"
)

func signalGroup(int, syscall.Signal) error { return errors.ErrUnsupported }
