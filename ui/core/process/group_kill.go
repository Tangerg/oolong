//go:build linux || dragonfly || freebsd || netbsd

package process

import "syscall"

// signalGroup needs no exit check here: these kernels signal every member that
// has not been reaped, unlike Darwin, which can exclude one already exiting.
func signalGroup(pid int, signal syscall.Signal) error {
	return syscall.Kill(-pid, signal)
}
