//go:build windows

package mermaid

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"

	"github.com/Tangerg/oolong/ui/core/process"
)

const terminatedProcessExitCode = 1

func platformSupported() error { return nil }

// Job membership precedes application code, so ordinary browser descendants
// remain supervised even when they detach or create nested jobs.
func run(ctx context.Context, cmd *exec.Cmd) (err error) {
	if cause := context.Cause(ctx); cause != nil {
		return cause
	}
	job, err := process.NewJob()
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, job.Close()) }()
	input, err := os.CreateTemp(cmd.Dir, "stdin-")
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, input.Close(), os.Remove(input.Name())) }()
	if err = fill(input, cmd.Stdin); err != nil {
		return err
	}
	output, writer, err := os.Pipe()
	if err != nil {
		return err
	}
	defer func() { _ = output.Close(); _ = writer.Close() }()
	child, err := job.Start(process.JobStartConfig{
		Path: cmd.Path, Args: cmd.Args, Dir: cmd.Dir,
		Stdin: input, Stdout: writer, Stderr: writer,
	})
	if err != nil {
		return err
	}
	if err = writer.Close(); err != nil {
		closeErr := job.Close()
		_, waitErr := child.Wait()
		return errors.Join(err, closeErr, waitErr)
	}
	copied := make(chan error, 1)
	go func() { _, copyErr := io.Copy(cmd.Stdout, output); copied <- copyErr }()
	return supervise(ctx, job, child, output, copied)
}

// Launcher completion does not prove that descendants have stopped, so every
// exit path settles the job before releasing its output owner.
func supervise(
	ctx context.Context,
	job *process.Job,
	child *os.Process,
	output *os.File,
	copied <-chan error,
) error {
	finished := make(chan struct{})
	cancelled := make(chan error, 1)
	go func() {
		select {
		case <-ctx.Done():
			cancelled <- job.Terminate(terminatedProcessExitCode)
		case <-finished:
			cancelled <- nil
		}
	}()
	state, waitErr := child.Wait()
	close(finished)
	cancelErr := <-cancelled
	terminateErr := job.Terminate(terminatedProcessExitCode)
	settledErr := job.Wait()
	if terminateErr != nil || settledErr != nil {
		// Something may still hold the pipe's other end, so the copier would never
		// see the end of it and the receive below would not return.
		settledErr = errors.Join(settledErr, output.Close())
	}
	copyErr := <-copied
	if waitErr == nil && !state.Success() {
		waitErr = &exec.ExitError{ProcessState: state}
	}
	return errors.Join(context.Cause(ctx), waitErr, cancelErr, terminateErr, settledErr, copyErr)
}

// An inherited handle starts at the file's current position, so prepared stdin
// must be rewound before process creation.
func fill(dst *os.File, src io.Reader) error {
	if _, err := io.Copy(dst, src); err != nil {
		return err
	}
	_, err := dst.Seek(0, io.SeekStart)
	return err
}
