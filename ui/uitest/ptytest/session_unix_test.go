//go:build darwin || linux

package ptytest_test

import (
	"context"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/Tangerg/oolong/ui/uitest/ptytest"
)

func TestWaitReservesTheLeaderIdentityUntilClose(t *testing.T) {
	needPTY(t)
	session, err := ptytest.Start(t.Context(), ptytest.Config{}, "sh", "-c", `printf 'descendant %d\n' "$$"`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = session.Close() }()
	waitFor(t, session.Transcript(), "descendant ")
	pid := reportedPID(t, session.Transcript().String())
	if err := session.Wait(t.Context()); err != nil {
		t.Fatal(err)
	}
	if !alive(pid) {
		t.Fatal("Wait released the leader's numeric identity before group cleanup")
	}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
}

// TestClosingEndsTheWholeSessionItStarted holds the harness to what it promised.
//
// Start gives the command a session of its own, so what a test walks away from is a
// process group. A close that killed only the leader would leave the rest of the
// group sitting on a terminal the test believes is gone, and holding that terminal
// open is also what makes the transcript reader's own read unendable.
func TestClosingEndsTheWholeSessionItStarted(t *testing.T) {
	needPTY(t)
	s, err := ptytest.Start(t.Context(), ptytest.Config{}, "sh", "-c",
		`(trap '' HUP; sleep 300) & printf 'descendant %d\n' "$!"; exec sleep 300`)
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, s.Transcript(), "descendant ")
	descendant := reportedPID(t, s.Transcript().String())

	closed := make(chan error, 1)
	go func() { closed <- s.Close() }()
	select {
	case <-closed:
	case <-time.After(20 * time.Second):
		t.Fatal("Close did not return while a descendant held the terminal")
	}

	// Reaping is the kernel's business and is not instantaneous, so the question is
	// whether the descendant goes, not whether it has already gone.
	deadline := time.Now().Add(10 * time.Second)
	for alive(descendant) {
		if time.Now().After(deadline) {
			t.Fatalf("process %d outlived the session that started it", descendant)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// TestClosingEndsTheSessionAProgramLeftBehind is the other half of it.
//
// A program that exits having left something of its own running has not ended the
// session: what it started is in the same group and holding the same terminal.
// Waiting for the leader answers a question about the leader, and reading it as an
// answer about the session let a close walk past everything the group still had in
// it.
func TestClosingEndsTheSessionAProgramLeftBehind(t *testing.T) {
	needPTY(t)
	s, err := ptytest.Start(t.Context(), ptytest.Config{}, "sh", "-c",
		`(trap '' HUP; sleep 300 & wait) & printf 'descendant %d\n' "$!"; exit 0`)
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, s.Transcript(), "descendant ")
	descendant := reportedPID(t, s.Transcript().String())
	if err := s.Wait(t.Context()); err != nil {
		t.Fatalf("waiting for the program: %v", err)
	}
	if !alive(descendant) {
		t.Skip("the descendant did not outlive the program that started it")
	}

	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for alive(descendant) {
		if time.Now().After(deadline) {
			t.Fatalf("process %d outlived the session that started it", descendant)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// The leader waits until its child has installed HUP handling before exiting.
// That leaves the replica open without relying on scheduler ordering.
func TestCancellationAfterTheLeaderExitsClosesTheSession(t *testing.T) {
	needPTY(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	s, err := ptytest.Start(ctx, ptytest.Config{Dir: t.TempDir()}, "sh", "-c",
		`(trap '' HUP; touch ready; exec sleep 300) & pid=$!; while ! test -e ready; do sleep 0.01; done; printf 'descendant %d\n' "$pid"`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	deadline, end := context.WithTimeout(t.Context(), 5*time.Second)
	defer end()
	waitFor(t, s.Transcript(), "descendant ")
	descendant := reportedPID(t, s.Transcript().String())
	if err := s.Wait(deadline); err != nil {
		t.Fatal(err)
	}
	// The descendant still holds the PTY. Its geometry can change after the
	// leader has exited, without sending a signal to that dead process.
	if err := s.Resize(ptytest.Size{Cols: 41, Rows: 13}); err != nil {
		t.Fatalf("resize after the session leader exited: %v", err)
	}
	cancel()
	if err := s.Drain(deadline); err != nil {
		t.Fatalf("context ended but the session is still holding the terminal: %v", err)
	}
	for alive(descendant) {
		select {
		case <-deadline.Done():
			t.Fatalf("descendant %d outlived cancellation", descendant)
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func reportedPID(t *testing.T, transcript string) int {
	t.Helper()
	_, rest, ok := strings.Cut(transcript, "descendant ")
	if !ok {
		t.Fatalf("the descendant did not report itself: %q", transcript)
	}
	digits := strings.TrimSpace(strings.SplitN(rest, "\n", 2)[0])
	pid, err := strconv.Atoi(digits)
	if err != nil || pid <= 1 {
		t.Fatalf("the descendant reported %q", digits)
	}
	return pid
}

// alive asks whether a process still exists without disturbing it. Signal zero is
// the standard way: the permission and existence checks run, the delivery does not.
func alive(pid int) bool {
	return syscall.Kill(pid, 0) == nil
}
