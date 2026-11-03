//go:build linux || dragonfly || freebsd || netbsd

package process

import "syscall"

func signalGroup(pid int, signal syscall.Signal) error {
	return syscall.Kill(-pid, signal)
}
