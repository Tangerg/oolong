//go:build unix

package mermaid

import (
	"context"
	"errors"
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
	// Puppeteer handles SIGINT by killing its detached browser group. WaitDelay
	// bounds an unresponsive CLI; settle remaining members of our own group too.
	killed := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	if errors.Is(killed, syscall.ESRCH) {
		killed = nil
	}
	return errors.Join(context.Cause(ctx), err, killed)
}
