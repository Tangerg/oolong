//go:build unix

package mermaid

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
	"testing"
)

func TestLostExitIdentityCannotAuthorizeAGroupSignal(t *testing.T) {
	cmd := exec.CommandContext(t.Context(), "sh", "-c", "read ending")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	input, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = input.Close() })
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	// The stale owner's numeric ID now names this controlled live group. Its
	// released handle cannot acquire either signaling or reaping authority.
	staleProcess, err := os.FindProcess(cmd.Process.Pid)
	if err != nil {
		t.Fatal(err)
	}
	stale := &exec.Cmd{Process: staleProcess}
	if err := finishGroup(stale, syscall.ECHILD); !errors.Is(err, syscall.ECHILD) {
		t.Fatalf("lost ownership was hidden: %v", err)
	}
	if _, err := input.Write([]byte("finish\n")); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatalf("cleanup acted on the replacement group: %v", err)
	}
}
