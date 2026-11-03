//go:build windows

package mermaid

import (
	"errors"
	"fmt"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	jobMessageNewProcess        = 6
	jobMessageActiveProcessZero = 4
	processHandleWaitTimeout    = time.Second
	jobTreeWaitTimeout          = 2 * time.Second
	jobStopKey                  = 0
	jobEventKey                 = 1
)

var isProcessInJob = windows.NewLazySystemDLL("kernel32.dll").NewProc("IsProcessInJob")

// Accounting reaches zero before the last process object is signaled. Retain
// process handles from NEW_PROCESS notifications and wait on those actual objects.
type jobTree struct {
	port windows.Handle
	job  windows.Handle
	done chan struct{}
	err  error
}

func newJobTree(job windows.Handle) (*jobTree, error) {
	port, err := windows.CreateIoCompletionPort(windows.InvalidHandle, 0, 0, 1)
	if err != nil {
		return nil, err
	}
	association := struct {
		Key  uintptr
		Port windows.Handle
	}{jobEventKey, port}
	if _, err = windows.SetInformationJobObject(job, windows.JobObjectAssociateCompletionPortInformation, uintptr(unsafe.Pointer(&association)), uint32(unsafe.Sizeof(association))); err != nil { //nolint:gosec // G103: exact native structure for this synchronous call.
		return nil, errors.Join(err, windows.CloseHandle(port))
	}
	tree := &jobTree{port: port, job: job, done: make(chan struct{})}
	go func() {
		tree.err = tree.observe()
		close(tree.done)
	}()
	return tree, nil
}

func (t *jobTree) observe() (err error) {
	var handles []windows.Handle
	var observed uint32
	defer func() {
		for _, h := range handles {
			err = errors.Join(err, windows.CloseHandle(h))
		}
	}()
	for {
		var message uint32
		var key uintptr
		// Job messages carry a numeric PID in the overlapped slot. Keep it in
		// integer storage so the GC never interprets a small PID as a Go pointer.
		var processID uintptr
		payload := (**windows.Overlapped)(unsafe.Pointer(&processID)) //nolint:gosec // G103: API-sized integer storage for the documented job-message payload.
		if err = windows.GetQueuedCompletionStatus(t.port, &message, &key, payload, windows.INFINITE); err != nil {
			return err
		}
		if key == jobStopKey {
			return nil
		}
		if handles, err = activeProcesses(handles); err != nil {
			return err
		}
		switch message {
		case jobMessageNewProcess:
			observed++
			if handles, err = observing(handles, t.job, uint32(processID)); err != nil {
				return err
			}
		case jobMessageActiveProcessZero:
			if err := t.verifyObserved(observed); err != nil {
				return err
			}
			return waitAll(handles)
		}
	}
}

// A process destroyed before its notification is handled needs no retained
// handle; other open failures must remain visible.
func observing(handles []windows.Handle, job windows.Handle, pid uint32) ([]windows.Handle, error) {
	h, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if errors.Is(err, windows.ERROR_INVALID_PARAMETER) {
		return handles, nil
	}
	if err != nil {
		return handles, fmt.Errorf("mermaid: observe process %d: %w", pid, err)
	}
	// A notification's PID may have been recycled. The opened object's immutable
	// membership, not the queued number, identifies whether this job owns its wait.
	var member int32
	result, _, queryErr := isProcessInJob.Call(uintptr(h), uintptr(job), uintptr(unsafe.Pointer(&member))) //nolint:gosec // G103: exact Win32 BOOL output retained until the synchronous call returns.
	if result == 0 {
		return handles, errors.Join(queryErr, windows.CloseHandle(h))
	}
	if member == 0 {
		return handles, windows.CloseHandle(h)
	}
	return append(handles, h), nil
}

func activeProcesses(handles []windows.Handle) ([]windows.Handle, error) {
	active := handles[:0]
	for index, handle := range handles {
		status, err := windows.WaitForSingleObject(handle, 0)
		if err == nil && status == windows.WAIT_OBJECT_0 {
			if err = windows.CloseHandle(handle); err == nil {
				continue
			}
		} else if err == nil && status != uint32(windows.WAIT_TIMEOUT) {
			err = fmt.Errorf("mermaid: process observation status %d", status)
		}
		if err != nil {
			return append(active, handles[index:]...), err
		}
		active = append(active, handle)
	}
	clear(handles[len(active):])
	return active, nil
}

func waitAll(handles []windows.Handle) error {
	deadline := time.Now().Add(processHandleWaitTimeout)
	for _, h := range handles {
		remaining := max(0, time.Until(deadline).Milliseconds())
		result, err := windows.WaitForSingleObject(h, uint32(remaining)) //nolint:gosec // G115: remaining is bounded by the one-second deadline.
		if err != nil {
			return err
		}
		if result != windows.WAIT_OBJECT_0 {
			return fmt.Errorf("mermaid: process wait status %d", result)
		}
	}
	return nil
}

func (t *jobTree) verifyObserved(observed uint32) error {
	// Notification delivery is not guaranteed under resource pressure. Never
	// report completion unless every process was observed or already destroyed.
	var accounting struct {
		User, Kernel, PeriodUser, PeriodKernel int64
		Faults, Total, Active, Terminated      uint32
	}
	//nolint:gosec // G103: documented accounting structure, exact size and lifetime.
	if queryErr := windows.QueryInformationJobObject(t.job, windows.JobObjectBasicAccountingInformation, uintptr(unsafe.Pointer(&accounting)), uint32(unsafe.Sizeof(accounting)), nil); queryErr != nil {
		return queryErr
	}
	if accounting.Total != observed || accounting.Active != 0 {
		return fmt.Errorf("mermaid: incomplete process notifications: observed %d of %d, active %d", observed, accounting.Total, accounting.Active)
	}

	return nil
}

func (t *jobTree) wait() error {
	timer := time.NewTimer(jobTreeWaitTimeout)
	defer timer.Stop()
	select {
	case <-t.done:
		return t.err
	case <-timer.C:
		return errors.New("mermaid: process tree did not finish shutdown")
	}
}

func (t *jobTree) close() error {
	err := windows.PostQueuedCompletionStatus(t.port, 0, jobStopKey, nil)
	if err != nil {
		// Closing the port wakes its observer even if posting the stop message failed.
		err = errors.Join(err, windows.CloseHandle(t.port))
		<-t.done
		return errors.Join(err, t.err)
	}
	<-t.done
	return errors.Join(t.err, windows.CloseHandle(t.port))
}
