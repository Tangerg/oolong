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
	file     *os.File
	mu       sync.Mutex
	deadline time.Time
}

var cancelSynchronousIO = windows.NewLazySystemDLL("kernel32.dll").NewProc("CancelSynchronousIo")

func newTerminalOutput(file *os.File) (*terminalOutput, error) {
	if err := cancelSynchronousIO.Find(); err != nil {
		return nil, err
	}
	return &terminalOutput{file: file}, nil
}
func (o *terminalOutput) active(bool) error { return nil }
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
		ticker := time.NewTicker(time.Millisecond)
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
