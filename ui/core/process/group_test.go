package process_test

import (
	"errors"
	"os"
	"syscall"
	"testing"

	"github.com/Tangerg/oolong/ui/core/process"
)

func TestSignalGroupRejectsAnUnownedLeader(t *testing.T) {
	for _, leader := range []*os.Process{nil, {Pid: -1}, {}, {Pid: 1}} {
		if err := process.SignalGroup(leader, syscall.SIGKILL); !errors.Is(err, os.ErrInvalid) {
			t.Fatalf("unowned process group: %v", err)
		}
	}
	if !process.Supported() {
		if err := process.SignalGroup(&os.Process{Pid: os.Getpid()}, syscall.SIGKILL); !errors.Is(err, errors.ErrUnsupported) {
			t.Fatalf("unsupported platform: %v", err)
		}
	}
}
