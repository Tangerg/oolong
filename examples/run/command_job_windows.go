//go:build windows

package main

import (
	"errors"
	"fmt"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	commandJobNewProcess        = 6
	commandJobActiveProcessZero = 4
	commandProcessWaitTimeout   = time.Second
	commandJobWaitTimeout       = 2 * time.Second
	commandJobStopKey           = 0
	commandJobEventKey          = 1
)

var isCommandProcessInJob = windows.NewLazySystemDLL("kernel32.dll").NewProc("IsProcessInJob")

// Job accounting can reach zero before the last process object is signaled.
// Retained process handles make completion about those objects, not reused PIDs.
type commandJob struct {
	handle windows.Handle
	port   windows.Handle
	done   chan struct{}
	err    error
}

func newCommandJob() (_ *commandJob, err error) {
	handle, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			err = errors.Join(err, windows.CloseHandle(handle))
		}
	}()
	limits := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	limits.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err = windows.SetInformationJobObject(handle, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits))); err != nil { //nolint:gosec // G103: documented native structure, exact size and lifetime.
		return nil, err
	}
	port, err := windows.CreateIoCompletionPort(windows.InvalidHandle, 0, 0, 1)
	if err != nil {
		return nil, err
	}
	association := struct {
		Key  uintptr
		Port windows.Handle
	}{commandJobEventKey, port}
	if _, err := windows.SetInformationJobObject(handle, windows.JobObjectAssociateCompletionPortInformation, uintptr(unsafe.Pointer(&association)), uint32(unsafe.Sizeof(association))); err != nil { //nolint:gosec // G103: exact native structure for this synchronous call.
		return nil, errors.Join(err, windows.CloseHandle(port))
	}
	job := &commandJob{handle: handle, port: port, done: make(chan struct{})}
	go func() {
		job.err = job.observe()
		close(job.done)
	}()
	return job, nil
}

func (j *commandJob) stop() error {
	return windows.TerminateJobObject(j.handle, commandTerminatedExitCode)
}

func (j *commandJob) observe() (err error) {
	var handles []windows.Handle
	var observed uint32
	defer func() {
		for _, handle := range handles {
			err = errors.Join(err, windows.CloseHandle(handle))
		}
	}()
	for {
		var message uint32
		var key, processID uintptr
		// The completion payload is a numeric PID, not a pointer for the GC to follow.
		payload := (**windows.Overlapped)(unsafe.Pointer(&processID)) //nolint:gosec // G103: API-sized integer storage for the documented job-message payload.
		if err = windows.GetQueuedCompletionStatus(j.port, &message, &key, payload, windows.INFINITE); err != nil {
			return err
		}
		if key == commandJobStopKey {
			return nil
		}
		if handles, err = activeCommandProcesses(handles); err != nil {
			return err
		}
		switch message {
		case commandJobNewProcess:
			observed++
			handle, owned, err := j.observeProcess(uint32(processID))
			if err != nil {
				return err
			}
			if owned {
				handles = append(handles, handle)
			}
		case commandJobActiveProcessZero:
			if err := j.verifyObserved(observed); err != nil {
				return err
			}
			return waitCommandProcesses(handles)
		}
	}
}

func (j *commandJob) observeProcess(pid uint32) (windows.Handle, bool, error) {
	handle, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if errors.Is(err, windows.ERROR_INVALID_PARAMETER) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("run: observe process %d: %w", pid, err)
	}
	// A queued PID may have been recycled before OpenProcess. Job membership is
	// immutable, so only the opened object's membership can authorize retaining it.
	var member int32
	result, _, queryErr := isCommandProcessInJob.Call(uintptr(handle), uintptr(j.handle), uintptr(unsafe.Pointer(&member))) //nolint:gosec // G103: exact Win32 BOOL output retained until the synchronous call returns.
	if result == 0 {
		return 0, false, errors.Join(queryErr, windows.CloseHandle(handle))
	}
	if member == 0 {
		return 0, false, windows.CloseHandle(handle)
	}
	return handle, true, nil
}

func activeCommandProcesses(handles []windows.Handle) ([]windows.Handle, error) {
	active := handles[:0]
	for index, handle := range handles {
		status, err := windows.WaitForSingleObject(handle, 0)
		if err == nil && status == windows.WAIT_OBJECT_0 {
			if err = windows.CloseHandle(handle); err == nil {
				continue
			}
		} else if err == nil && status != uint32(windows.WAIT_TIMEOUT) {
			err = fmt.Errorf("run: process observation status %d", status)
		}
		if err != nil {
			return append(active, handles[index:]...), err
		}
		active = append(active, handle)
	}
	clear(handles[len(active):])
	return active, nil
}

func (j *commandJob) verifyObserved(observed uint32) error {
	var accounting struct {
		User, Kernel, PeriodUser, PeriodKernel int64
		Faults, Total, Active, Terminated      uint32
	}
	// Notification delivery is not guaranteed under resource pressure; completion
	// requires evidence for every process the job actually admitted.
	if err := windows.QueryInformationJobObject(j.handle, windows.JobObjectBasicAccountingInformation, uintptr(unsafe.Pointer(&accounting)), uint32(unsafe.Sizeof(accounting)), nil); err != nil { //nolint:gosec // G103: documented accounting structure, exact size and lifetime.
		return err
	}
	if accounting.Total != observed || accounting.Active != 0 {
		return fmt.Errorf("run: incomplete process notifications: observed %d of %d, active %d", observed, accounting.Total, accounting.Active)
	}
	return nil
}

func waitCommandProcesses(handles []windows.Handle) error {
	deadline := time.Now().Add(commandProcessWaitTimeout)
	for _, handle := range handles {
		remaining := max(0, time.Until(deadline).Milliseconds())
		result, err := windows.WaitForSingleObject(handle, uint32(remaining)) //nolint:gosec // G115: remaining is bounded by the one-second deadline.
		if err != nil {
			return err
		}
		if result != windows.WAIT_OBJECT_0 {
			return fmt.Errorf("run: process wait status %d", result)
		}
	}
	return nil
}

func (j *commandJob) wait() error {
	timer := time.NewTimer(commandJobWaitTimeout)
	defer timer.Stop()
	select {
	case <-j.done:
		return j.err
	case <-timer.C:
		return errors.New("run: command job did not finish shutdown")
	}
}

func (j *commandJob) close() error {
	err := windows.PostQueuedCompletionStatus(j.port, 0, commandJobStopKey, nil)
	if err != nil {
		err = errors.Join(err, windows.CloseHandle(j.port))
		<-j.done
	} else {
		<-j.done
		err = windows.CloseHandle(j.port)
	}
	return errors.Join(err, j.err, windows.CloseHandle(j.handle))
}
