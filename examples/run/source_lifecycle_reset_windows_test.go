//go:build windows

package main

import (
	"errors"
	"syscall"
)

func connectionReset(err error) bool { return errors.Is(err, syscall.WSAECONNRESET) }
