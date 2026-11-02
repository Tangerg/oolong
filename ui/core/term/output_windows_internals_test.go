//go:build windows

package term

import (
	"errors"
	"os"
	"testing"

	"golang.org/x/sys/windows"
)

func TestTerminalOutputOwnsConsoleModeOnlyWhileActive(t *testing.T) {
	// An inactive screen buffer isolates the test from the runner's real display.
	create := windows.NewLazySystemDLL("kernel32.dll").NewProc("CreateConsoleScreenBuffer")
	const consoleTextmodeBuffer = 1
	h, _, err := create.Call(windows.GENERIC_READ|windows.GENERIC_WRITE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, 0, consoleTextmodeBuffer, 0)
	if windows.Handle(h) == windows.InvalidHandle {
		if errors.Is(err, windows.ERROR_INVALID_HANDLE) {
			t.Skip("a Windows console is required for a private screen buffer")
		}
		t.Fatal(err)
	}
	file := os.NewFile(h, "private console buffer")
	t.Cleanup(func() { _ = file.Close() })
	const original = windows.ENABLE_WRAP_AT_EOL_OUTPUT
	if modeErr := windows.SetConsoleMode(windows.Handle(h), original); modeErr != nil {
		t.Fatal(modeErr)
	}
	o, err := newTerminalOutput(file)
	if err != nil {
		t.Fatal(err)
	}
	assertMode := func(want uint32) {
		t.Helper()
		var mode uint32
		if err := windows.GetConsoleMode(windows.Handle(h), &mode); err != nil || mode != want {
			t.Fatalf("console mode = %#x, %v; want %#x", mode, err, want)
		}
	}
	const active = original | windows.ENABLE_PROCESSED_OUTPUT | windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING | windows.DISABLE_NEWLINE_AUTO_RETURN
	assertMode(active)
	if err := o.active(false); err != nil {
		t.Fatal(err)
	}
	assertMode(original)
	if err := o.active(true); err != nil {
		t.Fatal(err)
	}
	assertMode(active)
}
