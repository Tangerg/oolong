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
	exitingProcessFlag = 0x00002000
)

func signalGroup(pid int, signal syscall.Signal) error {
	err := unix.Kill(-pid, signal)
	if !errors.Is(err, unix.EPERM) {
		return err
	}
	// Darwin can exclude a process from group signaling before its public state
	// becomes SZOMB. P_WEXIT already proves irreversible exit. Inspect while the
	// unreaped leader still reserves this ID; reaping would permit group-ID reuse.
	members, inspectErr := unix.SysctlKinfoProcSlice(processGroupQuery, pid)
	if inspectErr != nil {
		return errors.Join(err, inspectErr)
	}
	if retainedExitingGroup(pid, members) {
		return nil
	}
	return err
}

func retainedExitingGroup(leaderPID int, members []unix.KinfoProc) bool {
	leaderFound := false
	for _, member := range members {
		if member.Proc.P_stat != zombieProcessState && member.Proc.P_flag&exitingProcessFlag == 0 {
			return false
		}
		leaderFound = leaderFound || int(member.Proc.P_pid) == leaderPID
	}
	return leaderFound
}
