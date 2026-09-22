//go:build unix

package mermaid

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"
)

func platformSupported() error { return nil }

func run(ctx context.Context, cmd *exec.Cmd) error {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGINT)
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}
		return err
	}
	cmd.WaitDelay = time.Second
	if err := cmd.Start(); err != nil {
		return err
	}
	err := cmd.Wait()
	// The owned launcher places the browser in this same group. Killing does not
	// depend on a JavaScript signal handler or a responsive rendering process.
	killed := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	waited := waitGroup(cmd.Process.Pid)
	if errors.Is(killed, syscall.ESRCH) || (errors.Is(killed, syscall.EPERM) && waited == nil) {
		killed = nil
	}
	return errors.Join(context.Cause(ctx), err, killed, waited)
}

func waitGroup(pid int) error {
	deadline := time.Now().Add(time.Second)
	for {
		err := syscall.Kill(-pid, 0)
		if errors.Is(err, syscall.ESRCH) {
			return nil
		}
		// Darwin can report EPERM while an exiting group's only members are
		// zombies. Only ESRCH proves the group is gone; a persistent denial
		// remains an error when the shutdown deadline expires.
		if err != nil && !errors.Is(err, syscall.EPERM) {
			return err
		}
		if !time.Now().Before(deadline) {
			return errors.Join(fmt.Errorf("mermaid: process group %d did not finish shutdown", pid), err)
		}
		time.Sleep(time.Millisecond)
	}
}
