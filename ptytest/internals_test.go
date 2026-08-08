package ptytest

// Tests of what this package keeps to itself.

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestATranscriptIsSafeToReadWhileItGrows(t *testing.T) {
	tr := newTranscript()
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := range 200 {
			tr.append([]byte{byte('a' + i%26)})
		}
	}()
	for range 200 {
		_ = tr.Bytes()
	}
	<-done
	if len(tr.Bytes()) != 200 {
		t.Fatalf("captured %d bytes, want 200", len(tr.Bytes()))
	}
}

func TestWaitForReturnsOnceEveryTokenHasArrived(t *testing.T) {
	tr := newTranscript()
	go func() {
		time.Sleep(10 * time.Millisecond)
		tr.append([]byte("first "))
		time.Sleep(10 * time.Millisecond)
		tr.append([]byte("second"))
	}()
	if err := tr.WaitWithin(2*time.Second, "first", "second"); err != nil {
		t.Fatalf("waiting for both: %v", err)
	}
}

func TestWaitForGivesUpAndSaysWhatItNeverSaw(t *testing.T) {
	tr := newTranscript()
	tr.append([]byte("only this"))

	err := tr.WaitWithin(20*time.Millisecond, "only", "missing")
	if err == nil {
		t.Fatal("waiting for something that never came did not fail")
	}
	if !strings.Contains(err.Error(), "missing") {
		t.Fatalf("the failure was %q, want it to name the token that never came", err)
	}
	if strings.Contains(err.Error(), `"only"`) {
		t.Fatalf("the failure was %q, want it to name only what was missing", err)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("the failure does not unwrap to a deadline: %v", err)
	}
}

func TestSizesThatATerminalCannotReportAreRefused(t *testing.T) {
	// Zero is refused at the door: a terminal collapsed to no size is a state a
	// test never means to ask for and always struggles to diagnose.
	for _, bad := range []Size{{0, 24}, {80, 0}, {-1, 24}, {1 << 20, 24}} {
		if err := bad.check(); err == nil {
			t.Errorf("%+v was accepted", bad)
		}
	}
	if err := (Size{80, 24}).check(); err != nil {
		t.Errorf("a normal size was refused: %v", err)
	}
}

func TestTheTranscriptIsAStringByteForByte(t *testing.T) {
	tr := newTranscript()
	tr.append([]byte("\x1b[0m\xff"))
	if got := tr.String(); got != "\x1b[0m\xff" {
		t.Fatalf("= %q, want the bytes unchanged", got)
	}
	if !strings.Contains(tr.String(), "\x1b[0m") {
		t.Fatal("an escape sequence did not survive the round trip")
	}
}

func TestATranscriptScreenUsesOneConcurrentSnapshot(t *testing.T) {
	tr := newTranscript()
	tr.append([]byte("first"))
	screen, err := tr.Screen(Size{Cols: 8, Rows: 1})
	if err != nil {
		t.Fatal(err)
	}
	tr.append([]byte(" later"))
	if got := strings.TrimRight(screen.Rows()[0], " "); got != "first" {
		t.Fatalf("snapshot changed after more output arrived: %q", got)
	}
}
