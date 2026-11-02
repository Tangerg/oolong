//go:build windows

package term

import (
	"context"
	"errors"
	"math"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"
	"unicode/utf8"
	"unsafe"

	"golang.org/x/sys/windows"
	xterm "golang.org/x/term"
)

func TestWakePrecedesReadyConsoleInput(t *testing.T) {
	readyEvent := func() windows.Handle {
		t.Helper()
		event, err := windows.CreateEvent(nil, 1, 1, nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = windows.CloseHandle(event) })
		return event
	}
	w := waker{input: readyEvent(), signal: readyEvent(), console: true}
	if ready, err := w.wait(); err != nil || ready {
		t.Fatalf("input overtook a pending wake: ready=%t err=%v", ready, err)
	}
	if ready, err := w.wait(); err != nil || !ready {
		t.Fatalf("input after wake: ready=%t err=%v", ready, err)
	}
}

func TestNonConsoleInputCannotStartAnUninterruptibleRead(t *testing.T) {
	event, err := windows.CreateEvent(nil, 1, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if closeErr := windows.CloseHandle(event); closeErr != nil {
			t.Error(closeErr)
		}
	}()
	w, err := newWaker(int(event))
	if err != nil {
		t.Fatal(err)
	}
	defer w.close()
	if _, err := w.read(nil); !errors.Is(err, errors.ErrUnsupported) {
		t.Fatalf("non-console read = %v, want unsupported", err)
	}
}

func TestConsoleInputUsesItsOwnNativeConsole(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestConsoleInputNativeHelper$", "-test.v") //nolint:gosec // G204: this test binary and fixed test-selection arguments are fixture-owned.
	cmd.Env = append(os.Environ(), "OOLONG_TERM_CONSOLE_TEST=1")
	// A separate console keeps injected records and mode changes away from the
	// developer's terminal. The timeout turns an accidentally blocking read into
	// a failing subprocess instead of hanging the test runner.
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NEW_CONSOLE, HideWindow: true}
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("native console test: %v\n%s", err, output)
	}
}

func TestConsoleInputNativeHelper(t *testing.T) {
	if os.Getenv("OOLONG_TERM_CONSOLE_TEST") != "1" {
		return
	}
	name, err := windows.UTF16PtrFromString("CONIN$")
	if err != nil {
		t.Fatal(err)
	}
	handle, err := windows.CreateFile(name, windows.GENERIC_READ|windows.GENERIC_WRITE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if closeErr := windows.CloseHandle(handle); closeErr != nil {
			t.Error(closeErr)
		}
	}()
	state, err := xterm.MakeRaw(int(handle))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if restoreErr := xterm.Restore(int(handle), state); restoreErr != nil {
			t.Error(restoreErr)
		}
	}()

	openInput := func(t *testing.T) *waker {
		t.Helper()
		if err := windows.FlushConsoleInputBuffer(handle); err != nil {
			t.Fatal(err)
		}
		w, err := newWaker(int(handle))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(w.close)
		return w
	}
	t.Run("noncharacter record", func(t *testing.T) {
		w := openInput(t)
		writeConsoleRecords(t, handle, []consoleInputRecord{{eventType: 0x0010}})
		if chunk, err := w.read(nil); err != nil || len(chunk) != 0 {
			t.Fatalf("focus record = %q, %v", chunk, err)
		}
		w.wake()
		if chunk, err := w.read(nil); err != nil || len(chunk) != 0 {
			t.Fatalf("wake after focus record = %q, %v", chunk, err)
		}
	})
	t.Run("wake preserves the next owner's input", func(t *testing.T) {
		w := openInput(t)
		writeConsoleRecords(t, handle, consoleCharacters("next"))
		w.wake()
		if chunk, err := w.read(nil); err != nil || len(chunk) != 0 {
			t.Fatalf("wake overtaken by input: %q, %v", chunk, err)
		}
		readConsoleText(t, w, "next")
	})
	t.Run("Unicode tail needs no later key", func(t *testing.T) {
		w := openInput(t)
		want := strings.Repeat("界", 2047) + "😀\x1a"
		writeConsoleRecords(t, handle, consoleCharacters(want))
		readConsoleText(t, w, want)
	})
	t.Run("repeats outlive the ready OS queue", func(t *testing.T) {
		w := openInput(t)
		var mode uint32
		if err := windows.GetConsoleMode(handle, &mode); err != nil {
			t.Fatal(err)
		}
		// A record queued before VT mode was enabled can carry a repeat count.
		// Preserve it without asking the now-empty kernel queue for another key.
		if err := windows.SetConsoleMode(handle, mode&^windows.ENABLE_VIRTUAL_TERMINAL_INPUT); err != nil {
			t.Fatal(err)
		}
		record := consoleCharacter('r')
		record.key.repeatCount = 6000
		writeConsoleRecords(t, handle, []consoleInputRecord{record})
		if err := windows.SetConsoleMode(handle, mode); err != nil {
			t.Fatal(err)
		}
		first, err := w.read(nil)
		if err != nil || len(first) == 0 || len(first) > readChunkBytes || !w.buffered() {
			t.Fatalf("first repeated chunk = %d bytes, %v, buffered=%t", len(first), err, w.buffered())
		}
		if firstText := string(first); firstText != strings.Repeat("r", len(first)) {
			t.Fatalf("first repeated chunk contains %q", firstText)
		}
		if err := windows.FlushConsoleInputBuffer(handle); err != nil {
			t.Fatal(err)
		}
		readConsoleText(t, w, strings.Repeat("r", 6000-len(first)))
	})
}

func writeConsoleRecords(t *testing.T, handle windows.Handle, records []consoleInputRecord) {
	t.Helper()
	if len(records) == 0 || uint64(len(records)) > math.MaxUint32 {
		t.Fatalf("console record count %d is outside the nonempty DWORD range", len(records))
	}
	proc := windows.NewLazySystemDLL("kernel32.dll").NewProc("WriteConsoleInputW")
	if err := proc.Find(); err != nil {
		t.Fatal(err)
	}
	var written uint32
	//nolint:gosec // G103: verified INPUT_RECORD layout and uint32 result storage remain live through this synchronous fixture call.
	result, _, err := proc.Call(uintptr(handle), uintptr(unsafe.Pointer(&records[0])),
		uintptr(len(records)), uintptr(unsafe.Pointer(&written)))
	if result == 0 || uint64(written) != uint64(len(records)) {
		t.Fatalf("write console records = %d/%d, %v", written, len(records), err)
	}
}

func readConsoleText(t *testing.T, w *waker, want string) {
	t.Helper()
	var got strings.Builder
	for got.Len() < len(want) {
		chunk, err := w.read(nil)
		if err != nil || len(chunk) > readChunkBytes || !utf8.Valid(chunk) {
			t.Fatalf("console chunk = %d bytes, %v", len(chunk), err)
		}
		got.Write(chunk)
	}
	if got.String() != want {
		t.Fatalf("console text = %q, want %q", got.String(), want)
	}
}
