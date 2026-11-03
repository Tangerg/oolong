//go:build windows

package main

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"

	"github.com/Tangerg/oolong/ui/core/process"
)

const commandTerminatedExitCode = 1

func (c *commandProcess) run(ctx context.Context) (err error) {
	job, err := process.NewJob()
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, job.Close()) }()
	input, err := os.Open(os.DevNull)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, input.Close()) }()
	output, writer, err := os.Pipe()
	if err != nil {
		return err
	}
	defer func() { _ = output.Close(); _ = writer.Close() }()
	if cause := context.Cause(ctx); cause != nil {
		return cause
	}
	c.cmd.Process, err = job.Start(process.JobStartConfig{
		Path: c.cmd.Path, Args: c.cmd.Args,
		Stdin: input, Stdout: writer, Stderr: writer,
	})
	if err != nil {
		return err
	}
	if err := writer.Close(); err != nil {
		closeErr := job.Close()
		_, waitErr := c.cmd.Process.Wait()
		return errors.Join(err, closeErr, waitErr)
	}
	copied := make(chan error, 1)
	go func() {
		_, err := io.Copy(c.cmd.Stdout, output)
		copied <- err
	}()
	return c.wait(ctx, job, output, copied)
}

func (c *commandProcess) wait(ctx context.Context, job *process.Job, output *os.File, copied <-chan error) error {
	finished := make(chan struct{})
	cancelled := make(chan error, 1)
	go func() {
		select {
		case <-ctx.Done():
			cancelled <- job.Terminate(commandTerminatedExitCode)
		case <-finished:
			cancelled <- nil
		}
	}()
	state, waitErr := c.cmd.Process.Wait()
	c.cmd.ProcessState = state
	close(finished)
	cancelErr := <-cancelled
	stopErr := job.Terminate(commandTerminatedExitCode)
	settledErr := job.Wait()
	if stopErr != nil || settledErr != nil {
		settledErr = errors.Join(settledErr, output.Close())
	}
	copyErr := <-copied
	if waitErr == nil && !state.Success() {
		waitErr = &exec.ExitError{ProcessState: state}
	}
	return errors.Join(context.Cause(ctx), waitErr, cancelErr, stopErr, settledErr, copyErr)
}
