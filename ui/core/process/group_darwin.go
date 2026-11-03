//go:build darwin

package process

import (
	"errors"
	"syscall"

	"golang.org/x/sys/unix"
)

const (
	processGroupQuery  = "kern.proc.pgrp"
	zombieProcessState = 5
)

func signalGroup(pid int, signal syscall.Signal) error {
	err := unix.Kill(-pid, signal)
	if !errors.Is(err, unix.EPERM) {
		return err
	}
	// Darwin excludes zombies from group signaling and reports EPERM if none
	// remain eligible. Inspect while the unreaped leader still reserves this ID;
	// a lookup after reaping could describe a different process group.
	members, inspectErr := unix.SysctlKinfoProcSlice(processGroupQuery, pid)
	if inspectErr != nil {
		return errors.Join(err, inspectErr)
	}
	if retainedZombieGroup(pid, members) {
		return nil
	}
	return err
}

func retainedZombieGroup(leaderPID int, members []unix.KinfoProc) bool {
	leaderFound := false
	for _, member := range members {
		if member.Proc.P_stat != zombieProcessState {
			return false
		}
		leaderFound = leaderFound || int(member.Proc.P_pid) == leaderPID
	}
	return leaderFound
}
