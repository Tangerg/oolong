//go:build darwin

package process

import (
	"testing"

	"golang.org/x/sys/unix"
)

func TestGroupExitEvidenceRequiresTheRetainedLeaderAndEveryMemberExiting(t *testing.T) {
	const (
		leaderPID            = 42
		descendantPID        = 43
		runnableProcessState = 2
		stoppedProcessState  = 4
		unknownProcessState  = 0
	)
	member := func(pid int32, state int8) unix.KinfoProc {
		return unix.KinfoProc{Proc: unix.ExternProc{P_pid: pid, P_stat: state}}
	}
	exiting := func(pid int32, state int8) unix.KinfoProc {
		result := member(pid, state)
		result.Proc.P_flag = 0x00002000 // Darwin's published P_WEXIT flag.
		return result
	}
	for _, test := range []struct {
		name    string
		members []unix.KinfoProc
		want    bool
	}{
		{name: "missing evidence"},
		{name: "another group", members: []unix.KinfoProc{member(descendantPID, zombieProcessState)}},
		{name: "retained leader", members: []unix.KinfoProc{member(leaderPID, zombieProcessState)}, want: true},
		{name: "exited descendants", members: []unix.KinfoProc{member(leaderPID, zombieProcessState), member(descendantPID, zombieProcessState)}, want: true},
		{name: "exiting leader", members: []unix.KinfoProc{exiting(leaderPID, runnableProcessState)}, want: true},
		{name: "exiting descendant", members: []unix.KinfoProc{member(leaderPID, zombieProcessState), exiting(descendantPID, runnableProcessState)}, want: true},
		{name: "exiting without retained leader", members: []unix.KinfoProc{exiting(descendantPID, runnableProcessState)}},
		{name: "exiting leader with live descendant", members: []unix.KinfoProc{exiting(leaderPID, runnableProcessState), member(descendantPID, runnableProcessState)}},
		{name: "live descendant", members: []unix.KinfoProc{member(leaderPID, zombieProcessState), member(descendantPID, runnableProcessState)}},
		{name: "stopped descendant", members: []unix.KinfoProc{member(leaderPID, zombieProcessState), member(descendantPID, stoppedProcessState)}},
		{name: "unknown state", members: []unix.KinfoProc{member(leaderPID, zombieProcessState), member(descendantPID, unknownProcessState)}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := retainedExitingGroup(leaderPID, test.members); got != test.want {
				t.Fatalf("group exit evidence = %v, want %v", got, test.want)
			}
		})
	}
}
