//go:build unix

package mermaid

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"syscall"
	"time"

	"github.com/Tangerg/oolong/ui/core/process"
)

const gracefulExitTimeout = time.Second

func platformSupported() error {
	if !process.Supported() {
		return errors.ErrUnsupported
	}
	return nil
}

// run reports what happened to the process group; the caller owns ctx and
// therefore reports its cause.
func run(ctx context.Context, backend launch) error {
	cmd := exec.Command(backend.path, backend.args...) //nolint:noctx,gosec // The group owner below controls cancellation and joins it before reaping; the backend is trusted configuration.
	cmd.Dir = backend.dir
	cmd.Stdin = backend.stdin
	cmd.Stdout, cmd.Stderr = backend.output, backend.output
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.WaitDelay = gracefulExitTimeout
	if err := cmd.Start(); err != nil {
		return err
	}
	finished := make(chan struct{})
	cancelled := make(chan error, 1)
	go func() { cancelled <- cancelGroup(ctx, cmd.Process, finished) }()
	observed := process.WaitExit(cmd.Process)
	close(finished)
	// Every group signal finishes before reaping can release its numeric identity.
	cancelErr := <-cancelled
	return errors.Join(cancelErr, finishGroup(cmd, observed))
}

func cancelGroup(ctx context.Context, leader *os.Process, finished <-chan struct{}) error {
	select {
	case <-finished:
		return nil
	case <-ctx.Done():
	}
	interrupted := signalBeforeExit(leader, syscall.SIGINT, finished)
	timer := time.NewTimer(gracefulExitTimeout)
	defer timer.Stop()
	select {
	case <-finished:
		return interrupted
	case <-timer.C:
		return errors.Join(interrupted, signalBeforeExit(leader, syscall.SIGKILL, finished))
	}
}

func finishGroup(cmd *exec.Cmd, observed error) error {
	if _, exited := errors.AsType[*process.ExitError](observed); observed != nil && !exited {
		// Failed observation cannot authorize another numeric signal or wait.
		// Release invalidates the handle; Wait then only joins Cmd's bounded pipes.
		released := cmd.Process.Release()
		_ = cmd.Wait()
		return errors.Join(observed, released)
	}
	// The unreaped launcher reserves the group ID. Non-child zombie reaping
	// belongs to the operating system, so no numeric lookup follows this wait.
	killed := process.SignalGroup(cmd.Process, syscall.SIGKILL)
	return errors.Join(killed, cmd.Wait())
}

func signalBeforeExit(leader *os.Process, signal syscall.Signal, finished <-chan struct{}) error {
	select {
	case <-finished:
		return nil
	default:
		return process.SignalGroup(leader, signal)
	}
}
