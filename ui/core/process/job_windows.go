//go:build windows

package process

import (
	"errors"
	"fmt"
	"os"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	jobMessageNewProcess        = 6
	jobMessageActiveProcessZero = 4
	processHandleWaitTimeout    = time.Second
	jobWaitTimeout              = 2 * time.Second
	jobStopKey                  = 0
	jobEventKey                 = 1
	jobTerminatedExitCode       = 1
)

var isProcessInJob = windows.NewLazySystemDLL("kernel32.dll").NewProc("IsProcessInJob")

var errJobObservationStopped = errors.New("process: job observation stopped before completion")

// Job owns one Windows launcher and its ordinary CreateProcess descendants,
// including nested jobs. Start installs membership before application code can
// run; breakaway is not enabled. Processes created by an unrelated broker remain
// outside this membership. The caller owns the launcher's final os.Process.Wait.
//
// Methods may run concurrently. Start admits at most one launcher. Close stops
// remaining work and releases the job. Job must not be copied; its zero value is
// not usable.
type Job struct {
	mu       sync.Mutex
	handle   windows.Handle
	port     windows.Handle
	started  bool
	closed   bool
	closeErr error
	done     chan struct{}
	err      error
}

// NewJob requires Windows 10 or Windows Server 2016 and later. It creates an empty
// containment owner; callers must Close it even if Start fails.
func NewJob() (_ *Job, err error) {
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
	}{jobEventKey, port}
	if _, err = windows.SetInformationJobObject(handle, windows.JobObjectAssociateCompletionPortInformation, uintptr(unsafe.Pointer(&association)), uint32(unsafe.Sizeof(association))); err != nil { //nolint:gosec // G103: exact native structure for this synchronous call.
		return nil, errors.Join(err, windows.CloseHandle(port))
	}
	job := &Job{handle: handle, port: port, done: make(chan struct{})}
	go func() {
		job.err = job.observe()
		close(job.done)
	}()
	return job, nil
}

// Terminate requests forced termination for every process in the job. It does
// not prove completion; Wait observes that separately. The caller chooses the
// exit code and when cancellation or a grace period warrants termination.
func (j *Job) Terminate(exitCode uint32) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	if err := j.usable(); err != nil {
		return err
	}
	return windows.TerminateJobObject(j.handle, exitCode)
}

// Wait proves that admitted processes have ended, including their process
// objects becoming signaled. It does not reap the launcher or terminate work.
// Call it after termination or natural completion; an incomplete observation
// after two seconds is an error. A job without an admitted launcher is invalid.
func (j *Job) Wait() error {
	j.mu.Lock()
	err := j.usable()
	started := j.started
	j.mu.Unlock()
	if err != nil {
		return err
	}
	if !started {
		return fmt.Errorf("process: wait for an unstarted job: %w", os.ErrInvalid)
	}
	return j.wait()
}

// Close terminates outstanding processes, waits for completion, and releases all
// job-owned handles. It also works before Start. Calls are idempotent and return
// the same cleanup result. The caller must still reap any returned launcher.
func (j *Job) Close() error {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.closed {
		return j.closeErr
	}
	if err := j.usable(); err != nil {
		return err
	}
	if j.started {
		j.closeErr = errors.Join(windows.TerminateJobObject(j.handle, jobTerminatedExitCode), j.wait())
	}
	j.closeErr = errors.Join(j.closeErr, j.closeObserver(), windows.CloseHandle(j.handle))
	j.closed = true
	return j.closeErr
}

func (j *Job) usable() error {
	if j.closed {
		return os.ErrClosed
	}
	if j.handle == 0 {
		return os.ErrInvalid
	}
	return nil
}

func (j *Job) observe() (err error) {
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
		// Job notifications carry a numeric PID, not a pointer for the GC to follow.
		payload := (**windows.Overlapped)(unsafe.Pointer(&processID)) //nolint:gosec // G103: API-sized integer storage for the documented job-message payload.
		if err = windows.GetQueuedCompletionStatus(j.port, &message, &key, payload, windows.INFINITE); err != nil {
			return err
		}
		if key == jobStopKey {
			return errJobObservationStopped
		}
		if handles, err = activeProcesses(handles); err != nil {
			return err
		}
		switch message {
		case jobMessageNewProcess:
			observed++
			handle, owned, observeErr := j.observeProcess(uint32(processID))
			if observeErr != nil {
				return observeErr
			}
			if owned {
				handles = append(handles, handle)
			}
		case jobMessageActiveProcessZero:
			if err := j.verifyObserved(observed); err != nil {
				return err
			}
			return waitProcesses(handles)
		}
	}
}

func (j *Job) observeProcess(pid uint32) (windows.Handle, bool, error) {
	handle, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if errors.Is(err, windows.ERROR_INVALID_PARAMETER) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("process: observe job process %d: %w", pid, err)
	}
	// A queued PID may have been recycled. The opened object's immutable job
	// membership, rather than the queued number, authorizes retaining its handle.
	var member int32
	result, _, queryErr := isProcessInJob.Call(uintptr(handle), uintptr(j.handle), uintptr(unsafe.Pointer(&member))) //nolint:gosec // G103: exact Win32 BOOL output retained until the synchronous call returns.
	if result == 0 {
		return 0, false, errors.Join(queryErr, windows.CloseHandle(handle))
	}
	if member == 0 {
		return 0, false, windows.CloseHandle(handle)
	}
	return handle, true, nil
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
			err = fmt.Errorf("process: job process observation status %d", status)
		}
		if err != nil {
			return append(active, handles[index:]...), err
		}
		active = append(active, handle)
	}
	clear(handles[len(active):])
	return active, nil
}

func (j *Job) verifyObserved(observed uint32) error {
	var accounting struct {
		User, Kernel, PeriodUser, PeriodKernel int64
		Faults, Total, Active, Terminated      uint32
	}
	// Notifications may be lost under resource pressure. Completion requires
	// evidence for every process admitted by the job, not merely an empty queue.
	if err := windows.QueryInformationJobObject(j.handle, windows.JobObjectBasicAccountingInformation, uintptr(unsafe.Pointer(&accounting)), uint32(unsafe.Sizeof(accounting)), nil); err != nil { //nolint:gosec // G103: documented accounting structure, exact size and lifetime.
		return err
	}
	if accounting.Total != observed || accounting.Active != 0 {
		return fmt.Errorf("process: incomplete job notifications: observed %d of %d, active %d", observed, accounting.Total, accounting.Active)
	}
	return nil
}

func waitProcesses(handles []windows.Handle) error {
	// Job accounting can reach zero before its final process object is signaled.
	deadline := time.Now().Add(processHandleWaitTimeout)
	for _, handle := range handles {
		remaining := max(0, time.Until(deadline).Milliseconds())
		status, err := windows.WaitForSingleObject(handle, uint32(remaining)) //nolint:gosec // G115: remaining is bounded by the one-second deadline.
		if err != nil {
			return err
		}
		if status != windows.WAIT_OBJECT_0 {
			return fmt.Errorf("process: job process wait status %d", status)
		}
	}
	return nil
}

func (j *Job) wait() error {
	timer := time.NewTimer(jobWaitTimeout)
	defer timer.Stop()
	select {
	case <-j.done:
		return j.err
	case <-timer.C:
		return errors.New("process: job did not finish shutdown")
	}
}

func (j *Job) closeObserver() error {
	err := windows.PostQueuedCompletionStatus(j.port, 0, jobStopKey, nil)
	if err != nil {
		// Closing the port wakes its observer even when posting the stop failed.
		err = errors.Join(err, windows.CloseHandle(j.port))
		<-j.done
	} else {
		<-j.done
		err = windows.CloseHandle(j.port)
	}
	// A started job's observation result was already reported by wait; joining it
	// again would report one failure twice.
	if j.started || errors.Is(j.err, errJobObservationStopped) {
		return err
	}
	return errors.Join(err, j.err)
}
