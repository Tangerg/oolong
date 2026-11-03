//go:build unix

package main

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
	"testing"
)

func TestLostCommandIdentityCannotAuthorizeAGroupSignal(t *testing.T) {
	command := exec.CommandContext(t.Context(), "sh", "-c", "read ending")
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	input, err := command.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = input.Close() })
	if err = command.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = command.Process.Kill(); _ = command.Wait() })
	stale, err := os.FindProcess(command.Process.Pid)
	if err != nil {
		t.Fatal(err)
	}
	owner := commandProcess{cmd: &exec.Cmd{Process: stale}}
	if err := owner.finish(syscall.ECHILD); !errors.Is(err, syscall.ECHILD) {
		t.Fatalf("lost wait ownership was hidden: %v", err)
	}
	if _, err := input.Write([]byte("finish\n")); err != nil {
		t.Fatal(err)
	}
	if err := command.Wait(); err != nil {
		t.Fatalf("stale cleanup signaled a process it no longer owned: %v", err)
	}
}
