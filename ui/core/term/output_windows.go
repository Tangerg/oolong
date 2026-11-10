//go:build windows

package term

import (
	"errors"
	"os"
	"runtime"
	"sync"
	"time"

	"golang.org/x/sys/windows"
)

// Windows console writes are synchronous. Pin each write to its OS thread so a
// shutdown deadline can cancel that exact I/O operation and join it before the
// display is returned to another owner.
type terminalOutput struct {
	file         *os.File
	originalMode uint32
	mu           sync.Mutex
	deadline     time.Time
	// moved is closed and replaced whenever the deadline changes, waking the
	// watcher of a write in progress to wait for the new deadline instead.
	moved chan struct{}
}

var cancelSynchronousIO = windows.NewLazySystemDLL("kernel32.dll").NewProc("CancelSynchronousIo")

// cancelRetryInterval paces repeated cancellation once a deadline has passed:
// CancelSynchronousIo affects only I/O already in progress, so a cancellation
// that lands just before the write starts must be tried again.
const cancelRetryInterval = time.Millisecond

func newTerminalOutput(file *os.File) (*terminalOutput, error) {
	if err := cancelSynchronousIO.Find(); err != nil {
		return nil, err
	}
	o := &terminalOutput{file: file, moved: make(chan struct{})}
	if err := windows.GetConsoleMode(windows.Handle(file.Fd()), &o.originalMode); err != nil {
		return nil, err
	}
	if err := o.active(true); err != nil {
		return nil, err
	}
	return o, nil
}

func (o *terminalOutput) active(active bool) error {
	mode := o.originalMode
	if active {
		// The renderer assumes VT parsing and delayed wrap at the right margin.
		mode |= windows.ENABLE_PROCESSED_OUTPUT | windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING |
			windows.ENABLE_WRAP_AT_EOL_OUTPUT | windows.DISABLE_NEWLINE_AUTO_RETURN
	}
	return windows.SetConsoleMode(windows.Handle(o.file.Fd()), mode)
}

func (o *terminalOutput) SetWriteDeadline(deadline time.Time) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.deadline = deadline
	close(o.moved)
	o.moved = make(chan struct{})
	return nil
}

func (o *terminalOutput) watch() (deadline time.Time, moved <-chan struct{}) {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.deadline, o.moved
}

// cancelAtDeadline cancels the pinned thread's write once the deadline passes,
// and keeps cancelling until the write ends. Until then it sleeps.
func (o *terminalOutput) cancelAtDeadline(thread windows.Handle, stop <-chan struct{}) {
	if !o.awaitDeadline(stop) {
		return
	}
	retry := time.NewTicker(cancelRetryInterval)
	defer retry.Stop()
	for {
		_, _, _ = cancelSynchronousIO.Call(uintptr(thread))
		select {
		case <-stop:
			return
		case <-retry.C:
		}
	}
}

// awaitDeadline reports whether the deadline passed before stop, following the
// deadline wherever SetWriteDeadline moves it.
func (o *terminalOutput) awaitDeadline(stop <-chan struct{}) bool {
	for {
		deadline, moved := o.watch()
		if deadline.IsZero() {
			select {
			case <-stop:
				return false
			case <-moved:
				continue
			}
		}
		timer := time.NewTimer(time.Until(deadline))
		select {
		case <-stop:
			timer.Stop()
			return false
		case <-moved:
			timer.Stop()
		case <-timer.C:
			return true
		}
	}
}

func (o *terminalOutput) WriteString(value string) (int, error) { return o.Write([]byte(value)) }
func (o *terminalOutput) Write(value []byte) (n int, err error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	thread, err := windows.OpenThread(windows.THREAD_TERMINATE, false, windows.GetCurrentThreadId())
	if err != nil {
		return 0, err
	}
	defer func() { err = errors.Join(err, windows.CloseHandle(thread)) }()
	stop, done := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(done)
		o.cancelAtDeadline(thread, stop)
	}()
	if deadline, _ := o.watch(); !deadline.IsZero() && !time.Now().Before(deadline) {
		close(stop)
		<-done
		return 0, os.ErrDeadlineExceeded
	}
	n, err = o.file.Write(value)
	close(stop)
	<-done
	if errors.Is(err, windows.ERROR_OPERATION_ABORTED) {
		err = os.ErrDeadlineExceeded
	}
	return n, err
}
func (o *terminalOutput) Close() error { return nil }
