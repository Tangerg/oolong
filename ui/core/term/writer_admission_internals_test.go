package term

import (
	"errors"
	"io"
	"testing"
)

// A failed reacquisition used to leave admission paused for a handover that had
// ended, so every later frame reported ErrHandoverActive instead of the failure.
func TestASealedWriterReportsItsFirstCause(t *testing.T) {
	w := NewWriter(WriterConfig{Destination: io.Discard})
	t.Cleanup(func() { _ = w.Close() })
	if err := w.pauseAdmission(); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Queue([]byte("frame")); !errors.Is(err, ErrHandoverActive) {
		t.Fatalf("paused queue = %v", err)
	}
	cause := errors.New("raw mode refused")
	w.seal(cause)
	w.resumeAdmission()
	w.CloseAdmission()
	if _, err := w.Queue([]byte("frame")); !errors.Is(err, cause) {
		t.Fatalf("sealed queue = %v, want the reacquisition failure", err)
	}
	if err := w.pauseAdmission(); !errors.Is(err, cause) {
		t.Fatalf("handover over a sealed writer = %v", err)
	}
	if _, err := w.QueueControl([]byte("leave")); err != nil {
		t.Fatalf("control output after a seal = %v, want the restore path open", err)
	}
}
