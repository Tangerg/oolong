package process_test

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
	"testing"

	"github.com/Tangerg/oolong/ui/core/process"
)

func TestWaitExitPreservesTheChildForItsReapingOwner(t *testing.T) {
	if !process.Supported() {
		t.Skip("non-reaping observation is unsupported")
	}
	for _, test := range []struct {
		command string
		code    int
		signal  os.Signal
	}{
		{command: "exit 0"},
		{command: "exit 7", code: 7},
		{command: "kill -TERM $$", code: -1, signal: syscall.SIGTERM},
	} {
		t.Run(test.command, func(t *testing.T) {
			cmd := exec.CommandContext(t.Context(), "sh", "-c", test.command) //nolint:gosec // G204: the table contains only fixed test-owned shell programs.
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
			for range 2 {
				err := process.WaitExit(cmd.Process)
				if test.code == 0 {
					if err != nil {
						t.Fatal(err)
					}
					continue
				}
				exit, exited := errors.AsType[*process.ExitError](err)
				if !exited || exit.Code != test.code || exit.Signal != test.signal || exit.Error() == "" {
					t.Fatalf("exit=%v; want code=%d signal=%v", err, test.code, test.signal)
				}
			}
			if err := cmd.Wait(); test.code == 0 && err != nil {
				t.Fatalf("observing exit stole the final wait: %v", err)
			}
			if err := process.WaitExit(cmd.Process); err == nil {
				t.Fatal("reaped child still appeared waitable")
			}
		})
	}
}

func TestWaitExitRejectsAnUnownedProcess(t *testing.T) {
	for _, child := range []*os.Process{nil, {}} {
		if err := process.WaitExit(child); !errors.Is(err, os.ErrInvalid) {
			t.Fatalf("invalid process: %v", err)
		}
	}
	if !process.Supported() {
		if err := process.WaitExit(&os.Process{Pid: os.Getpid()}); !errors.Is(err, errors.ErrUnsupported) {
			t.Fatalf("unsupported platform: %v", err)
		}
	}
}
