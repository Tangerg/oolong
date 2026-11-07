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
}

var cancelSynchronousIO = windows.NewLazySystemDLL("kernel32.dll").NewProc("CancelSynchronousIo")

const writeDeadlinePollInterval = time.Millisecond

func newTerminalOutput(file *os.File) (*terminalOutput, error) {
	if err := cancelSynchronousIO.Find(); err != nil {
		return nil, err
	}
	o := &terminalOutput{file: file}
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
	o.deadline = deadline
	o.mu.Unlock()
	return nil
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
		ticker := time.NewTicker(writeDeadlinePollInterval)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				o.mu.Lock()
				expired := !o.deadline.IsZero() && !time.Now().Before(o.deadline)
				o.mu.Unlock()
				if expired {
					_, _, _ = cancelSynchronousIO.Call(uintptr(thread))
				}
			}
		}
	}()
	o.mu.Lock()
	expired := !o.deadline.IsZero() && !time.Now().Before(o.deadline)
	o.mu.Unlock()
	if expired {
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
