//go:build windows

package term

import (
	"errors"
	"fmt"
	"os"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Console readiness may describe records without text. Nonblocking record
// reads avoid uncancellable ReadConsole waits; owning UTF-16 decoding keeps
// buffered characters available after the OS queue empties.
type waker struct {
	// input is the console this session reads, and console says it is one. A session
	// whose input is a pipe cannot supply cancellable console input and is refused
	// by read. OpenOn checks this before creating a session.
	input   windows.Handle
	console bool
	// signal is set when this process wants the reader back. It is manual-reset, so a
	// wake-up that arrives while nobody is waiting is still there when somebody does.
	signal windows.Handle

	mu     sync.Mutex
	closed bool

	records [128]consoleInputRecord
	text    consoleText
}

var readConsoleInputEx = windows.NewLazySystemDLL("kernel32.dll").NewProc("ReadConsoleInputExW")

// INPUT_RECORD's tagged union starts at byte four and occupies sixteen bytes.
// Check the native layout at compile time before passing it across the syscall.
var (
	_ [20]byte = [unsafe.Sizeof(consoleInputRecord{})]byte{}
	_ [4]byte  = [unsafe.Offsetof(consoleInputRecord{}.key)]byte{}
)

func newWaker(fd int) (*waker, error) {
	if err := readConsoleInputEx.Find(); err != nil {
		return nil, fmt.Errorf("term: find nonblocking console input: %w", err)
	}
	input := windows.Handle(fd)
	var mode uint32
	console := windows.GetConsoleMode(input, &mode) == nil

	event, err := windows.CreateEvent(nil, 1, 0, nil)
	if err != nil {
		return nil, fmt.Errorf("term: an event to wake the reader: %w", err)
	}
	return &waker{input: input, console: console, signal: event}, nil
}

func (w *waker) interruptible() bool { return w.console }

func (w *waker) buffered() bool { return len(w.text.records) > 0 }

func (w *waker) read(_ *os.File) readResult {
	if !w.console {
		return readResult{at: time.Now(), err: fmt.Errorf("term: input is not a console: %w", errors.ErrUnsupported)}
	}
	if w.buffered() {
		return w.text.read()
	}
	ready, err := w.wait()
	if err != nil || !ready {
		return readResult{at: time.Now(), err: err}
	}
	var count uint32
	const consoleReadNowait = 0x0002
	//nolint:gosec // G103: the checked INPUT_RECORD layout and uint32 count remain live through this synchronous kernel call.
	result, _, cause := readConsoleInputEx.Call(
		uintptr(w.input),
		uintptr(unsafe.Pointer(&w.records[0])),
		uintptr(len(w.records)),
		uintptr(unsafe.Pointer(&count)),
		consoleReadNowait,
	)
	at := time.Now()
	if result == 0 {
		if errors.Is(cause, windows.ERROR_SUCCESS) {
			cause = errors.New("console read failed without an error code")
		}
		return readResult{at: at, err: fmt.Errorf("term: read console input: %w", cause)}
	}
	if count > uint32(len(w.records)) {
		return readResult{at: at, err: fmt.Errorf("term: console returned %d records into a %d-record buffer", count, len(w.records))}
	}
	w.text.records = w.records[:count]
	w.text.at = at
	return w.text.read()
}

func (w *waker) reset() { w.text.reset() }

func (w *waker) wait() (bool, error) {
	if !w.console {
		return false, fmt.Errorf("term: input is not a console: %w", errors.ErrUnsupported)
	}
	// A wait-any selects the lowest ready index. A handover already requested
	// must take precedence over removing more records from the console.
	which, err := windows.WaitForMultipleObjects([]windows.Handle{w.signal, w.input}, false, windows.INFINITE)
	if err != nil {
		return false, fmt.Errorf("term: watch the terminal: %w", err)
	}
	switch which {
	case windows.WAIT_OBJECT_0:
		// Manual-reset, so it is cleared here rather than by the wait: one wake-up
		// must not answer the next one as well.
		_ = windows.ResetEvent(w.signal)
		return false, nil
	case windows.WAIT_OBJECT_0 + 1:
		return true, nil
	default:
		return false, fmt.Errorf("term: watch the terminal: the wait ended with %d", which)
	}
}

func (w *waker) wake() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return
	}
	_ = windows.SetEvent(w.signal)
}

// close releases the event. It belongs to the reader and is called when the reader
// returns, for the reason it is everywhere else: closing a handle out from under a
// wait is closing a number the operating system is free to hand out again.
func (w *waker) close() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return
	}
	w.closed = true
	_ = windows.CloseHandle(w.signal)
}
