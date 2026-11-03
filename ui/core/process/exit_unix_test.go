//go:build unix

package process_test

import (
	"context"
	"errors"
	"os/exec"
	"syscall"
	"testing"
	"time"

	"github.com/Tangerg/oolong/ui/core/process"
)

func TestStoppedChildRemainsUnfinished(t *testing.T) {
	if !process.Supported() {
		t.Skip("non-reaping observation is unsupported")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	// A separate child group makes SIGTSTP stop only this child, and BSD's
	// WaitStatus helpers distinguish that stop from continuation.
	cmd := exec.CommandContext(ctx, "sh", "-c", "kill -TSTP $$; exit 0")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	var status syscall.WaitStatus
	for {
		pid, err := syscall.Wait4(cmd.Process.Pid, &status, syscall.WUNTRACED, nil)
		if errors.Is(err, syscall.EINTR) {
			continue
		}
		if err != nil || pid != cmd.Process.Pid || !status.Stopped() || status.StopSignal() != syscall.SIGTSTP {
			t.Fatalf("child did not stop: pid=%d status=%v error=%v", pid, status, err)
		}
		break
	}
	observed := make(chan error, 1)
	go func() { observed <- process.WaitExit(cmd.Process) }()
	select {
	case err := <-observed:
		t.Fatalf("a stopped child was reported finished: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	if err := cmd.Process.Signal(syscall.SIGCONT); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-observed:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("continued child did not finish")
	}
}
