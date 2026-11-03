//go:build darwin || linux || dragonfly || freebsd || netbsd

package process_test

import (
	"bufio"
	"errors"
	"io"
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"

	"github.com/Tangerg/oolong/ui/core/process"
)

const processGroupTestTimeout = 5 * time.Second

func TestSignalGroupAcceptsAnExitedUnreapedLeader(t *testing.T) {
	command := exec.CommandContext(t.Context(), "sh", "-c", "exit 0")
	startProcessGroup(t, command)
	if err := process.WaitExit(command.Process); err != nil {
		t.Fatal(err)
	}
	if err := process.SignalGroup(command.Process, syscall.SIGKILL); err != nil {
		t.Fatalf("completed group needed no further signal: %v", err)
	}
	if err := process.WaitExit(command.Process); err != nil {
		t.Fatalf("signaling consumed the leader's exit: %v", err)
	}
}

func TestSignalGroupTerminatesDescendantsBeforeReapingTheLeader(t *testing.T) {
	command := exec.CommandContext(t.Context(), "sh", "-c", "sh -c 'printf \"ready\\n\"; exec sleep 60' & wait")
	output := startProcessGroup(t, command)
	reader := bufio.NewReader(output)
	if ready, err := reader.ReadString('\n'); err != nil || ready != "ready\n" {
		t.Fatalf("descendant startup = %q, %v", ready, err)
	}
	if err := process.SignalGroup(command.Process, syscall.SIGKILL); err != nil {
		t.Fatal(err)
	}
	// The descendant inherits this pipe. EOF proves it released its descriptor
	// without consulting a numeric PID that its own parent might already reap.
	if _, err := io.ReadAll(reader); err != nil {
		t.Fatalf("descendant retained the group's output after termination: %v", err)
	}
	err := process.WaitExit(command.Process)
	exit, exited := errors.AsType[*process.ExitError](err)
	if !exited || exit.Signal != syscall.SIGKILL {
		t.Fatalf("leader exit = %v, want SIGKILL", err)
	}
}

func startProcessGroup(t *testing.T, command *exec.Cmd) *os.File {
	t.Helper()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reader.Close(); _ = writer.Close() })
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	command.Stdout = writer
	if err = command.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
		_ = command.Wait()
	})
	if err = writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err = reader.SetReadDeadline(time.Now().Add(processGroupTestTimeout)); err != nil {
		t.Fatal(err)
	}
	return reader
}
