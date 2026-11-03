//go:build windows

package main

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"unsafe"

	"golang.org/x/sys/windows"
)

const commandTerminatedExitCode = 1

func (c *commandProcess) run(ctx context.Context) (err error) {
	job, err := newCommandJob()
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, job.close()) }()
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
	c.cmd.Process, err = job.start(c.cmd, input, writer)
	if err != nil {
		if c.cmd.Process == nil {
			return err
		}
		return job.abandon(c.cmd.Process, err)
	}
	if err := writer.Close(); err != nil {
		return job.abandon(c.cmd.Process, err)
	}
	copied := make(chan error, 1)
	go func() {
		_, err := io.Copy(c.cmd.Stdout, output)
		copied <- err
	}()
	return c.wait(ctx, job, output, copied)
}

func (c *commandProcess) wait(ctx context.Context, job *commandJob, output *os.File, copied <-chan error) error {
	finished := make(chan struct{})
	cancelled := make(chan error, 1)
	go func() {
		select {
		case <-ctx.Done():
			cancelled <- job.stop()
		case <-finished:
			cancelled <- nil
		}
	}()
	state, waitErr := c.cmd.Process.Wait()
	c.cmd.ProcessState = state
	close(finished)
	cancelErr := <-cancelled
	stopErr := job.stop()
	settledErr := job.wait()
	if stopErr != nil || settledErr != nil {
		settledErr = errors.Join(settledErr, output.Close())
	}
	copyErr := <-copied
	if waitErr == nil && !state.Success() {
		waitErr = &exec.ExitError{ProcessState: state}
	}
	return errors.Join(context.Cause(ctx), waitErr, cancelErr, stopErr, settledErr, copyErr)
}

func (j *commandJob) start(cmd *exec.Cmd, input, output *os.File) (_ *os.Process, err error) {
	attributes, err := windows.NewProcThreadAttributeList(2)
	if err != nil {
		return nil, err
	}
	defer attributes.Delete()
	var handles []windows.Handle
	defer func() {
		for _, handle := range handles {
			err = errors.Join(err, windows.CloseHandle(handle))
		}
	}()
	for _, file := range []*os.File{input, output} {
		var handle windows.Handle
		current := windows.CurrentProcess()
		if err = windows.DuplicateHandle(current, windows.Handle(file.Fd()), current, &handle, 0, true, windows.DUPLICATE_SAME_ACCESS); err != nil {
			return nil, err
		}
		handles = append(handles, handle)
	}
	if err = attributes.Update(windows.PROC_THREAD_ATTRIBUTE_HANDLE_LIST, unsafe.Pointer(&handles[0]), uintptr(len(handles))*unsafe.Sizeof(handles[0])); err != nil { //nolint:gosec // G103: the attribute container retains the inherited handles until CreateProcess returns.
		return nil, err
	}
	// Windows 10 installs job membership during creation, before command code can
	// launch a descendant. Assigning a job after Start would leave an escape window.
	const jobListAttribute = 0x0002000d
	if err = attributes.Update(jobListAttribute, unsafe.Pointer(&j.handle), unsafe.Sizeof(j.handle)); err != nil { //nolint:gosec // G103: a live job handle retained by the attribute container.
		return nil, err
	}
	startup := windows.StartupInfoEx{ProcThreadAttributeList: attributes.List()}
	startup.Cb = uint32(unsafe.Sizeof(startup))
	startup.Flags = windows.STARTF_USESTDHANDLES
	startup.StdInput, startup.StdOutput, startup.StdErr = handles[0], handles[1], handles[1]
	// Cmd.Start normally applies Windows executable extensions from private state.
	// Native creation needs the resolved path in lpApplicationName itself.
	path, err := exec.LookPath(cmd.Path)
	if err != nil {
		return nil, err
	}
	executable, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	commandLine, err := windows.UTF16PtrFromString(windows.ComposeCommandLine(cmd.Args))
	if err != nil {
		return nil, err
	}
	var info windows.ProcessInformation
	if err = windows.CreateProcess(executable, commandLine, nil, nil, true, windows.EXTENDED_STARTUPINFO_PRESENT|windows.CREATE_NO_WINDOW, nil, nil, &startup.StartupInfo, &info); err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, windows.CloseHandle(info.Thread), windows.CloseHandle(info.Process)) }()
	child, err := os.FindProcess(int(info.ProcessId))
	if err != nil {
		return nil, errors.Join(err, j.stop(), j.wait())
	}
	return child, nil
}

func (j *commandJob) abandon(child *os.Process, cause error) error {
	stopErr := j.stop()
	_, waitErr := child.Wait()
	return errors.Join(cause, stopErr, waitErr, j.wait())
}
