package ptytest

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// These drive the harness itself against small commands, so that the parts a
// terminal program exercises incidentally are stated somewhere on purpose.

func needPTY(t *testing.T) {
	t.Helper()
	if !Supported() {
		t.Skip("no pty on this platform")
	}
}

func TestASessionCapturesWhatTheCommandWrote(t *testing.T) {
	needPTY(t)
	s, err := Start("echo", "hello from a pty")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	if err := s.Transcript().WaitWithin(10*time.Second, "hello from a pty"); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := s.Wait(ctx); err != nil {
		t.Fatalf("waiting for a command that exits cleanly: %v", err)
	}
}

func TestASessionSendsWhatIsTyped(t *testing.T) {
	needPTY(t)
	// cat echoes its input back through the pty, which is the smallest thing that
	// proves typing reaches the far end.
	s, err := Start("cat")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	if err := s.Type("knock knock\n"); err != nil {
		t.Fatal(err)
	}
	if err := s.Transcript().WaitWithin(10*time.Second, "knock knock"); err != nil {
		t.Fatal(err)
	}
}

func TestASessionReportsTheSizeItWasGiven(t *testing.T) {
	needPTY(t)
	s, err := StartWith(Options{Size: Size{Cols: 37, Rows: 11}}, "stty", "size")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	// stty prints "rows cols".
	if err := s.Transcript().WaitWithin(10*time.Second, "11 37"); err != nil {
		t.Fatalf("the pty was not opened at the size it was asked for: %v", err)
	}
}

func TestResizingIsRefusedForASizeATerminalCannotReport(t *testing.T) {
	needPTY(t)
	s, err := Start("cat")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	if err := s.Resize(Size{Cols: 0, Rows: 0}); err == nil {
		t.Fatal("a terminal was collapsed to no size")
	}
}

func TestStartingSomethingThatIsNotThereFails(t *testing.T) {
	needPTY(t)
	if _, err := Start("this-command-does-not-exist-anywhere"); err == nil {
		t.Fatal("starting a command that does not exist succeeded")
	}
}

func TestStartingAtASizeATerminalCannotReportFails(t *testing.T) {
	if _, err := StartWith(Options{Size: Size{Cols: -1, Rows: 5}}, "echo"); err == nil {
		t.Fatal("a pty was opened at a size that cannot be represented")
	}
}

func TestClosingTwiceIsSafe(t *testing.T) {
	// So a test can defer it and still close explicitly.
	needPTY(t)
	s, err := Start("cat")
	if err != nil {
		t.Fatal(err)
	}
	_ = s.Close()
	_ = s.Close()
}

func TestClosingKillsSomethingStillRunning(t *testing.T) {
	needPTY(t)
	s, err := Start("cat") // waits for input for ever
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() { defer close(done); _ = s.Close() }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("closing a session did not stop the program it was running")
	}
}

func TestWaitGivesUpWhenItsContextDoes(t *testing.T) {
	needPTY(t)
	s, err := Start("cat")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if err := s.Wait(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("waiting on a program that never exits gave %v, want a deadline", err)
	}
}

func TestWriteSendsRawBytes(t *testing.T) {
	needPTY(t)
	s, err := Start("cat")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	n, err := s.Write([]byte("raw\n"))
	if err != nil || n != 4 {
		t.Fatalf("wrote %d bytes, %v", n, err)
	}
	if err := s.Transcript().WaitWithin(10*time.Second, "raw"); err != nil {
		t.Fatal(err)
	}
}

func TestAnUnsupportedPlatformSaysSoRatherThanHanging(t *testing.T) {
	if Supported() {
		t.Skip("this platform has a pty")
	}
	_, err := Start("echo")
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("= %v, want ErrUnsupported", err)
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
