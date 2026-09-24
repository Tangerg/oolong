//go:build windows

package mermaid

import (
	"errors"
	"fmt"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

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
	}{1, port}
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
		if key == 0 {
			return nil
		}
		switch message {
		case 6: // JOB_OBJECT_MSG_NEW_PROCESS: value carries the process ID, not a pointer.
			observed++
			if handles, err = observing(handles, uint32(processID)); err != nil {
				return err
			}
		case 4: // JOB_OBJECT_MSG_ACTIVE_PROCESS_ZERO
			if err := t.verifyObserved(observed); err != nil {
				return err
			}
			return waitAll(handles)
		}
	}
}

// observing adds the handle of a process the job has just reported. One that exited
// and was destroyed before this could reach it is not an error and adds nothing.
func observing(handles []windows.Handle, pid uint32) ([]windows.Handle, error) {
	h, err := windows.OpenProcess(windows.SYNCHRONIZE, false, pid)
	if errors.Is(err, windows.ERROR_INVALID_PARAMETER) {
		return handles, nil
	}
	if err != nil {
		return handles, fmt.Errorf("mermaid: observe process %d: %w", pid, err)
	}
	return append(handles, h), nil
}

// waitAll waits for every observed process to exit, within one second in total.
func waitAll(handles []windows.Handle) error {
	deadline := time.Now().Add(time.Second)
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
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	select {
	case <-t.done:
		return t.err
	case <-timer.C:
		return errors.New("mermaid: process tree did not finish shutdown")
	}
}

func (t *jobTree) close() error {
	err := windows.PostQueuedCompletionStatus(t.port, 0, 0, nil)
	if err != nil {
		// Closing the port wakes its observer even if posting the stop message failed.
		err = errors.Join(err, windows.CloseHandle(t.port))
		<-t.done
		return errors.Join(err, t.err)
	}
	<-t.done
	return errors.Join(t.err, windows.CloseHandle(t.port))
}
