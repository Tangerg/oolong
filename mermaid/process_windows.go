//go:build windows

package mermaid

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"
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
	input, err := os.CreateTemp(cmd.Dir, "stdin-")
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, input.Close(), os.Remove(input.Name())) }()
	if _, err = io.Copy(input, cmd.Stdin); err != nil {
		return err
	}
	if _, err = input.Seek(0, io.SeekStart); err != nil {
		return err
	}
	output, writer, err := os.Pipe()
	if err != nil {
		return err
	}
	defer func() { _ = output.Close(); _ = writer.Close() }()
	process, err := startInJob(cmd, job, input, writer)
	if err != nil {
		if process != nil {
			stopErr := windows.TerminateJobObject(job, 1)
			_, waitErr := process.Wait()
			err = errors.Join(err, stopErr, waitErr, settleJob(job))
		}
		return err
	}
	if err = writer.Close(); err != nil {
		stopErr := windows.TerminateJobObject(job, 1)
		_, waitErr := process.Wait()
		return errors.Join(err, stopErr, waitErr, settleJob(job))
	}
	copied := make(chan error, 1)
	go func() { _, copyErr := io.Copy(cmd.Stdout, output); copied <- copyErr }()
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
	settledErr := settleJob(job)
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
		return nil, errors.Join(err, windows.TerminateJobObject(job, 1), settleJob(job))
	}
	return process, nil
}

func settleJob(job windows.Handle) error {
	var accounting struct {
		TotalUserTime, TotalKernelTime, PeriodUserTime, PeriodKernelTime int64
		PageFaults, TotalProcesses, ActiveProcesses, TerminatedProcesses uint32
	}
	for {
		if err := windows.QueryInformationJobObject(job, windows.JobObjectBasicAccountingInformation, uintptr(unsafe.Pointer(&accounting)), uint32(unsafe.Sizeof(accounting)), nil); err != nil { //nolint:gosec // G103: documented JOBOBJECT_BASIC_ACCOUNTING_INFORMATION layout and exact size.
			return fmt.Errorf("mermaid: settle process tree: %w", err)
		}
		if accounting.ActiveProcesses == 0 {
			return nil
		}
		time.Sleep(time.Millisecond)
	}
}
