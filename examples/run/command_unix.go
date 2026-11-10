//go:build unix

package main

import (
	"context"
	"errors"
	"syscall"
	"time"

	"github.com/Tangerg/oolong/ui/core/process"
)

const commandOutputDrainTimeout = time.Second

func (c *commandProcess) run(ctx context.Context) error {
	if !process.Supported() {
		return errors.ErrUnsupported
	}
	c.cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	c.cmd.WaitDelay = commandOutputDrainTimeout
	if err := c.cmd.Start(); err != nil {
		return err
	}
	finished := make(chan struct{})
	cancelled := make(chan error, 1)
	go func() {
		select {
		case <-ctx.Done():
			cancelled <- process.SignalGroup(c.cmd.Process, syscall.SIGKILL)
		case <-finished:
			cancelled <- nil
		}
	}()
	observed := process.WaitExit(c.cmd.Process)
	close(finished)
	cancelErr := <-cancelled
	return errors.Join(cancelErr, c.finish(observed))
}

func (c *commandProcess) finish(observed error) error {
	if _, exited := errors.AsType[*process.ExitError](observed); observed != nil && !exited {
		// Lost wait ownership cannot authorize signaling a possibly reused group ID.
		released := c.cmd.Process.Release()
		_ = c.cmd.Wait()
		return errors.Join(observed, released)
	}
	// The unreaped leader reserves the group ID until the final signal completes.
	stopped := process.SignalGroup(c.cmd.Process, syscall.SIGKILL)
	return errors.Join(stopped, c.cmd.Wait())
}
