//go:build windows

package term

import (
	"testing"

	"golang.org/x/sys/windows"
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
