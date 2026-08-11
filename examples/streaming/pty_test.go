package main

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/oolong/ptytest"
)

// These are the tests a program.Host cannot write. A host proves the interface
// drew the frame it meant to; only a real terminal proves the bytes of that frame
// do what they were supposed to — that the session gives the terminal back, that
// what was printed reached the scrollback, that an idle interface goes quiet.

// build compiles the example once and returns the path to it.
func build(t *testing.T) string {
	t.Helper()
	binary := filepath.Join(t.TempDir(), "streaming")
	if os.PathSeparator == '\\' {
		binary += ".exe"
	}
	// The only variable here is a path this test just made with t.TempDir.
	//nolint:gosec // G204: the arguments are this test's own, not a caller's.
	out, err := exec.CommandContext(t.Context(), "go", "build", "-o", binary, ".").CombinedOutput()
	if err != nil {
		t.Fatalf("building the example: %v\n%s", err, out)
	}
	return binary
}

// start runs the example on a pty, skipping where there is no pty to run it on.
//
// The skip comes before the build, not after: skipping at the end of the
// expensive part still costs what it skipped, once per test.
func start(t *testing.T) *ptytest.Session {
	t.Helper()
	if !ptytest.Supported() {
		t.Skip("no pty on this platform")
	}
	session, err := ptytest.Start(
		t.Context(),
		ptytest.Config{Size: openingSize},
		build(t),
	)
	if errors.Is(err, ptytest.ErrUnsupported) {
		t.Skip("no pty on this platform")
	}
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}

var openingSize = ptytest.Size{Cols: 60, Rows: 20}

// settle is how long to wait for something to reach the terminal.
//
// Generous on purpose. A test that passes does not wait, and the machine running
// this is often building the rest of the suite at the same time — a real pty
// under a loaded runner is the one place in this repository where being patient
// costs nothing and being brisk costs a false failure.
const settle = 30 * time.Second

func waitFor(t *testing.T, s *ptytest.Session, tokens ...string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), settle)
	defer cancel()
	if err := s.Transcript().WaitFor(ctx, tokens...); err != nil {
		t.Fatal(err)
	}
}

// quiesce waits until the interface has stopped writing, and returns how much it
// had written by then.
//
// Waiting for a fixed interval instead would be the flaky version: on a loaded
// machine the opening frames can still be arriving when the measurement starts,
// and what that measures is the machine rather than the interface.
func quiesce(t *testing.T, s *ptytest.Session) int {
	t.Helper()
	const still = 250 * time.Millisecond
	deadline := time.Now().Add(settle)
	last, quietSince := -1, time.Now()
	for time.Now().Before(deadline) {
		switch n := len(s.Transcript().Bytes()); {
		case n != last:
			last, quietSince = n, time.Now()
		case time.Since(quietSince) >= still:
			return n
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("the interface was still writing after %v", settle)
	return 0
}

func TestTheInterfaceIsUpBeforeAnybodyTypes(t *testing.T) {
	s := start(t)
	waitFor(t, s, "Ask something")
	quiesce(t, s)
	screen, err := s.Transcript().Screen(openingSize)
	if err != nil {
		t.Fatal(err)
	}
	if shown := strings.Join(screen.Rows(), "\n"); !strings.Contains(shown, "Ask something") {
		t.Fatalf("opening screen does not contain the composer placeholder:\n%s", shown)
	}
}

func TestTypingAndSendingReachesTheTerminal(t *testing.T) {
	s := start(t)
	waitFor(t, s, "Ask something")
	if _, err := io.WriteString(s, "what is this"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, s, "what is this")
	if _, err := io.WriteString(s, "\r"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, s, "Approve prompt")
	if _, err := io.WriteString(s, "\r"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, s, "you", "Streaming first")
}

func TestTheSessionGivesTheTerminalBack(t *testing.T) {
	// The failure this catches is the one the user cannot recover from without
	// closing the window, and nothing else in the suite can see it.
	s := start(t)
	waitFor(t, s, "Ask something")
	if _, err := io.WriteString(s, "\x03"); err != nil { // ctrl+c, which the example binds to quit
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), settle)
	defer cancel()
	if err := s.Wait(ctx); err != nil {
		t.Fatalf("the program did not exit: %v", err)
	}

	transcript := s.Transcript().Bytes()
	ptytest.RequireSymmetricModes(t, transcript,
		ptytest.Mode{Name: "bracketed paste", On: "\x1b[?2004h", Off: "\x1b[?2004l"},
	)
	// An inline interface never takes the alternate screen. Taking it would put
	// everything the program printed somewhere the user cannot scroll back to,
	// which is the entire point of this mode.
	ptytest.RequireNotContains(t, transcript, "\x1b[?1049h")
	// And the cursor is left visible, or the shell after it has an invisible one.
	ptytest.RequireContains(t, transcript, "\x1b[?25h")
}

func TestAnIdleInterfaceGoesQuiet(t *testing.T) {
	// A frame that would change nothing writes nothing, so an interface nobody is
	// touching is silent on the wire and the cursor keeps blinking.
	s := start(t)
	waitFor(t, s, "Ask something")
	settled := quiesce(t, s)
	time.Sleep(700 * time.Millisecond)
	if grew := len(s.Transcript().Bytes()) - settled; grew != 0 {
		t.Fatalf("an idle interface wrote %d more bytes, want silence", grew)
	}
}

func TestResizingRepaintsRatherThanCorrupting(t *testing.T) {
	s := start(t)
	waitFor(t, s, "Ask something")
	before := quiesce(t, s)
	if err := s.Resize(ptytest.Size{Cols: 40, Rows: 20}); err != nil {
		t.Fatal(err)
	}
	// The block repaints in full from where the cursor was left, so something has
	// to arrive. What it must not do is stop redrawing.
	deadline := time.Now().Add(settle)
	for time.Now().Before(deadline) {
		if len(s.Transcript().Bytes()) > before {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("the interface never repainted after a resize")
}

func TestWhatWasPrintedSurvivesTheProgram(t *testing.T) {
	// The claim the library is built around: what has been said belongs to the
	// terminal, and is still there after the program that said it has gone.
	s := start(t)
	waitFor(t, s, "Ask something")
	if _, err := io.WriteString(s, "hello there\r"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, s, "Approve prompt")
	if _, err := io.WriteString(s, "\r"); err != nil {
		t.Fatal(err)
	}
	// Waiting for the final retained paragraph also means earlier stable blocks have
	// crossed the bounded window into terminal-owned scrollback.
	waitFor(t, s, "complete", "When this paragraph finishes")
	if _, err := io.WriteString(s, "\x03"); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), settle)
	defer cancel()
	if err := s.Wait(ctx); err != nil {
		t.Fatal(err)
	}
	// Nothing after the last frame erases the screen or the scrollback: leaving is
	// not the same as cleaning up after yourself.
	tail := s.Transcript().String()
	for _, erase := range []string{"\x1b[2J", "\x1b[3J"} {
		if strings.Contains(tail, erase) {
			t.Fatalf("the session erased the terminal with %q on its way out", erase)
		}
	}
}
