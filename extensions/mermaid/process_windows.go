//go:build windows

package mermaid

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"unsafe"

	"golang.org/x/sys/windows"
)

func platformSupported() error { return nil }

// Job membership is installed by CreateProcess, before any application code can
// spawn a browser. Neither detached children nor nested browser jobs escape it.
func run(ctx context.Context, cmd *exec.Cmd) (err error) {
	if cause := context.Cause(ctx); cause != nil {
		return cause
	}
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, windows.CloseHandle(job)) }()
	limits := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	limits.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err = windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits))); err != nil { //nolint:gosec // G103: documented Windows job structure, exact size, live until the synchronous call returns.
		return err
	}
	tree, err := newJobTree(job)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, tree.close()) }()
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
	process, err := startInJob(cmd, job, input, writer)
	if err != nil {
		if process == nil {
			return err
		}
		return abandon(job, process, tree, err)
	}
	if err = writer.Close(); err != nil {
		return abandon(job, process, tree, err)
	}
	copied := make(chan error, 1)
	go func() { _, copyErr := io.Copy(cmd.Stdout, output); copied <- copyErr }()
	return supervise(ctx, job, tree, process, output, copied)
}

// supervise waits for the process, ends the job whether the process finished or the
// context did, and joins everything that went wrong on the way.
func supervise(
	ctx context.Context,
	job windows.Handle,
	tree *jobTree,
	process *os.Process,
	output *os.File,
	copied <-chan error,
) error {
	finished := make(chan struct{})
	cancelled := make(chan error, 1)
	go func() {
		select {
		case <-ctx.Done():
			cancelled <- windows.TerminateJobObject(job, 1)
		case <-finished:
			cancelled <- nil
		}
	}()
	state, waitErr := process.Wait()
	close(finished)
	cancelErr := <-cancelled
	terminateErr := windows.TerminateJobObject(job, 1)
	settledErr := tree.wait()
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

func startInJob(cmd *exec.Cmd, job windows.Handle, input, output *os.File) (_ *os.Process, err error) {
	attributes, err := windows.NewProcThreadAttributeList(2)
	if err != nil {
		return nil, err
	}
	defer attributes.Delete()
	handles := []windows.Handle{windows.Handle(input.Fd()), windows.Handle(output.Fd())}
	defer func() {
		for _, handle := range handles {
			err = errors.Join(err, windows.SetHandleInformation(handle, windows.HANDLE_FLAG_INHERIT, 0))
		}
	}()
	for _, handle := range handles {
		if err = windows.SetHandleInformation(handle, windows.HANDLE_FLAG_INHERIT, windows.HANDLE_FLAG_INHERIT); err != nil {
			return nil, err
		}
	}
	if err = attributes.Update(windows.PROC_THREAD_ATTRIBUTE_HANDLE_LIST, unsafe.Pointer(&handles[0]), uintptr(len(handles))*unsafe.Sizeof(handles[0])); err != nil { //nolint:gosec // G103: the attribute container retains the handle array until CreateProcess returns.
		return nil, err
	}
	// PROC_THREAD_ATTRIBUTE_JOB_LIST is available on Windows 10 and later.
	const jobListAttribute = 0x0002000d
	if err = attributes.Update(jobListAttribute, unsafe.Pointer(&job), unsafe.Sizeof(job)); err != nil { //nolint:gosec // G103: a single job handle retained by the attribute container.
		return nil, err
	}
	startup := windows.StartupInfoEx{ProcThreadAttributeList: attributes.List()}
	startup.Cb = uint32(unsafe.Sizeof(startup))
	startup.Flags = windows.STARTF_USESTDHANDLES
	startup.StdInput, startup.StdOutput, startup.StdErr = handles[0], handles[1], handles[1]
	executable, err := windows.UTF16PtrFromString(cmd.Path)
	if err != nil {
		return nil, err
	}
	commandLine, err := windows.UTF16PtrFromString(windows.ComposeCommandLine(cmd.Args))
	if err != nil {
		return nil, err
	}
	directory, err := windows.UTF16PtrFromString(cmd.Dir)
	if err != nil {
		return nil, err
	}
	var info windows.ProcessInformation
	if err = windows.CreateProcess(executable, commandLine, nil, nil, true, windows.EXTENDED_STARTUPINFO_PRESENT|windows.CREATE_NO_WINDOW, nil, directory, &startup.StartupInfo, &info); err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, windows.CloseHandle(info.Thread), windows.CloseHandle(info.Process)) }()
	process, err := os.FindProcess(int(info.ProcessId))
	if err != nil {
		return nil, errors.Join(err, windows.TerminateJobObject(job, 1))
	}
	return process, nil
}

// fill copies src into dst and rewinds it, which is what a handle the job inherits
// has to be: a started process reads from the file's own position.
func fill(dst *os.File, src io.Reader) error {
	if _, err := io.Copy(dst, src); err != nil {
		return err
	}
	_, err := dst.Seek(0, io.SeekStart)
	return err
}

// abandon ends the job and waits for what it started, so a launch that failed part of
// the way through leaves nothing running behind it.
func abandon(job windows.Handle, process *os.Process, tree *jobTree, cause error) error {
	stopErr := windows.TerminateJobObject(job, 1)
	_, waitErr := process.Wait()
	return errors.Join(cause, stopErr, waitErr, tree.wait())
}
