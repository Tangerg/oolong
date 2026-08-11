package ptytest_test

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/Tangerg/oolong/ptytest"
)

// These drive the harness itself against small commands, so that the parts a
// terminal program exercises incidentally are stated somewhere on purpose.

func needPTY(t *testing.T) {
	t.Helper()
	if !ptytest.Supported() {
		t.Skip("no pty on this platform")
	}
}

func waitFor(t *testing.T, transcript *ptytest.Transcript, tokens ...string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	if err := transcript.WaitFor(ctx, tokens...); err != nil {
		t.Fatal(err)
	}
}

func TestASessionCapturesWhatTheCommandWrote(t *testing.T) {
	needPTY(t)
	s, err := ptytest.Start(t.Context(), ptytest.Config{}, "echo", "hello from a pty")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()

	waitFor(t, s.Transcript(), "hello from a pty")
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	if err := s.Wait(ctx); err != nil {
		t.Fatalf("waiting for a command that exits cleanly: %v", err)
	}
}

func TestASessionSendsWhatIsTyped(t *testing.T) {
	needPTY(t)
	// cat echoes its input back through the pty, which is the smallest thing that
	// proves typing reaches the far end.
	s, err := ptytest.Start(t.Context(), ptytest.Config{}, "cat")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()

	if _, err := io.WriteString(s, "typed and echoed\n"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, s.Transcript(), "typed and echoed")
}

func TestASessionReportsTheSizeItWasGiven(t *testing.T) {
	needPTY(t)
	s, err := ptytest.Start(t.Context(), ptytest.Config{Size: ptytest.Size{Cols: 37, Rows: 11}}, "stty", "size")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()

	// stty prints "rows cols".
	waitFor(t, s.Transcript(), "11 37")
}

func TestResizingIsRefusedForASizeATerminalCannotReport(t *testing.T) {
	needPTY(t)
	s, err := ptytest.Start(t.Context(), ptytest.Config{}, "cat")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()

	if err := s.Resize(ptytest.Size{Cols: 0, Rows: 0}); err == nil {
		t.Fatal("a terminal was collapsed to no size")
	}
}

func TestStartingSomethingThatIsNotThereFails(t *testing.T) {
	needPTY(t)
	if _, err := ptytest.Start(t.Context(), ptytest.Config{}, "this-command-does-not-exist-anywhere"); err == nil {
		t.Fatal("starting a command that does not exist succeeded")
	}
}

func TestStartingAtASizeATerminalCannotReportFails(t *testing.T) {
	if _, err := ptytest.Start(t.Context(), ptytest.Config{Size: ptytest.Size{Cols: -1, Rows: 5}}, "echo"); err == nil {
		t.Fatal("a pty was opened at a size that cannot be represented")
	}
}

func TestClosingTwiceIsSafe(t *testing.T) {
	// So a test can defer it and still close explicitly.
	needPTY(t)
	s, err := ptytest.Start(t.Context(), ptytest.Config{}, "cat")
	if err != nil {
		t.Fatal(err)
	}
	_ = s.Close()
	_ = s.Close()
}

func TestClosingKillsSomethingStillRunning(t *testing.T) {
	needPTY(t)
	s, err := ptytest.Start(t.Context(), ptytest.Config{}, "cat") // waits for input for ever
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
	s, err := ptytest.Start(t.Context(), ptytest.Config{}, "cat")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Millisecond)
	defer cancel()
	if err := s.Wait(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("waiting on a program that never exits gave %v, want a deadline", err)
	}
}

func TestWaitPreservesTheContextsCause(t *testing.T) {
	needPTY(t)
	s, err := ptytest.Start(t.Context(), ptytest.Config{}, "cat")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()

	want := errors.New("test stopped the conversation")
	ctx, cancel := context.WithCancelCause(t.Context())
	cancel(want)
	if err := s.Wait(ctx); !errors.Is(err, want) {
		t.Fatalf("waiting with a cause gave %v, want %v", err, want)
	}
}

func TestWriteSendsRawBytes(t *testing.T) {
	needPTY(t)
	s, err := ptytest.Start(t.Context(), ptytest.Config{}, "cat")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()

	n, err := s.Write([]byte("raw\n"))
	if err != nil || n != 4 {
		t.Fatalf("wrote %d bytes, %v", n, err)
	}
	waitFor(t, s.Transcript(), "raw")
}

func TestAnUnsupportedPlatformSaysSoRatherThanHanging(t *testing.T) {
	if ptytest.Supported() {
		t.Skip("this platform has a pty")
	}
	_, err := ptytest.Start(t.Context(), ptytest.Config{}, "echo")
	if !errors.Is(err, ptytest.ErrUnsupported) {
		t.Fatalf("= %v, want ptytest.ErrUnsupported", err)
	}
}
