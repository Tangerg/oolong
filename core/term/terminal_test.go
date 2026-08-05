package term_test

import (
	"os"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"

	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/term"
)

// A terminal is the one thing here that cannot be stood in for, so these open a
// real one. What is asserted is the lifecycle: that taking it over says what it
// means to, that it comes back the way it was found, and that the parts a caller
// touches work on a terminal that is not this process's own.

// pty opens a pty pair and closes both ends when the test is done.
func pty(t *testing.T) (primary, replica *os.File) {
	t.Helper()
	fd, err := unix.Open("/dev/ptmx", unix.O_RDWR|unix.O_CLOEXEC, 0)
	if err != nil {
		t.Skipf("no pty here: %v", err)
	}
	name, err := ptyName(fd)
	if err != nil {
		_ = unix.Close(fd)
		t.Skipf("no pty here: %v", err)
	}
	rfd, err := unix.Open(name, unix.O_RDWR|unix.O_NOCTTY|unix.O_CLOEXEC, 0)
	if err != nil {
		_ = unix.Close(fd)
		t.Skipf("no pty here: %v", err)
	}
	// Non-blocking so the runtime's poller owns it, which is what makes a read
	// deadline work: a plain descriptor would block a test for ever instead.
	if err := unix.SetNonblock(fd, true); err != nil {
		_ = unix.Close(fd)
		_ = unix.Close(rfd)
		t.Skipf("no pty here: %v", err)
	}
	primary = os.NewFile(uintptr(fd), "/dev/ptmx")
	replica = os.NewFile(uintptr(rfd), name)
	t.Cleanup(func() { _ = replica.Close(); _ = primary.Close() })
	return primary, replica
}

func open(t *testing.T, opts term.Options) (*term.Terminal, *os.File) {
	t.Helper()
	primary, replica := pty(t)
	// The replica is the terminal, so that is the side the session takes over and
	// the primary is where a test watches from.
	tty, err := term.OpenOn(replica, replica, opts)
	if err != nil {
		t.Fatalf("opening a pty as a terminal: %v", err)
	}
	t.Cleanup(func() { _ = tty.Close() })
	return tty, primary
}

func TestOpeningSomethingThatIsNotATerminal(t *testing.T) {
	// The case a caller has to handle rather than force: a program whose output is
	// being piped wants to write text, not frames.
	f, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	if _, err := term.OpenOn(f, f, term.Options{}); err == nil {
		t.Fatal("a file that is not a terminal was taken over")
	}
}

func TestASessionSaysWhatItIsTurningOn(t *testing.T) {
	tty, watch := open(t, term.Options{Mouse: true, Focus: true, Keyboard: true})

	seen := read(t, watch, 500*time.Millisecond)
	for name, seq := range map[string]string{
		"mouse":    "\x1b[?1003h",
		"focus":    "\x1b[?1004h",
		"keyboard": "\x1b[>31u",
		"paste":    "\x1b[?2004h",
	} {
		if !strings.Contains(seen, seq) {
			t.Errorf("%s was never turned on: %q", name, seen)
		}
	}
	// And not the alternate screen, which was not asked for.
	if strings.Contains(seen, "\x1b[?1049h") {
		t.Errorf("the alternate screen was taken without being asked for: %q", seen)
	}
	_ = tty
}

func TestASessionGivesTheTerminalBack(t *testing.T) {
	tty, watch := open(t, term.Options{Mouse: true})
	read(t, watch, 300*time.Millisecond)

	if err := tty.Close(); err != nil {
		t.Fatalf("closing: %v", err)
	}
	seen := read(t, watch, 500*time.Millisecond)
	if !strings.Contains(seen, "\x1b[?1003l") {
		t.Errorf("the mouse was never released: %q", seen)
	}
	if !strings.Contains(seen, "\x1b[?2004l") {
		t.Errorf("bracketed paste was never turned off: %q", seen)
	}
	// Closing twice is safe, including from a deferred call on a failed path.
	if err := tty.Close(); err != nil {
		t.Fatalf("closing twice: %v", err)
	}
}

func TestATerminalReportsItsSizeAndItsOpeningResize(t *testing.T) {
	tty, _ := open(t, term.Options{})

	w, h, err := tty.Size()
	if err != nil {
		t.Fatalf("asking the size: %v", err)
	}
	if w < 0 || h < 0 {
		t.Fatalf("size = %dx%d", w, h)
	}
	// The size arrives as an event too, so a session learns it the same way it
	// learns about every later change.
	select {
	case ev := <-tty.Events():
		if _, ok := ev.(input.Resize); !ok {
			t.Fatalf("first event = %#v, want the opening size", ev)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the opening size never arrived as an event")
	}
}

func TestATerminalsWriterReachesIt(t *testing.T) {
	tty, watch := open(t, term.Options{})
	read(t, watch, 200*time.Millisecond)

	s := grid.NewScreen(4, 1)
	s.Frame().Text(0, 0, "hi", grid.Style{})
	var frame frameBytes
	if err := s.Flush(&frame); err != nil {
		t.Fatal(err)
	}
	tty.Writer().Queue(frame.b)
	if !tty.Writer().Drain(2 * time.Second) {
		t.Fatal("the frame never reached the terminal")
	}
	if seen := read(t, watch, 500*time.Millisecond); !strings.Contains(seen, "hi") {
		t.Fatalf("the terminal was sent %q, want the frame in it", seen)
	}
}

func TestKeystrokesTypedAtATerminalArriveAsEvents(t *testing.T) {
	tty, watch := open(t, term.Options{})
	<-tty.Events() // the opening size

	if _, err := watch.Write([]byte("k")); err != nil {
		t.Fatal(err)
	}
	select {
	case ev := <-tty.Events():
		key, ok := ev.(input.Key)
		if !ok || key.Rune != 'k' {
			t.Fatalf("got %#v, want the keystroke", ev)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the keystroke never arrived")
	}
}

// read drains whatever the terminal has been sent, up to a deadline.
func read(t *testing.T, f *os.File, within time.Duration) string {
	t.Helper()
	if err := f.SetReadDeadline(time.Now().Add(within)); err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	buf := make([]byte, 4096)
	for {
		n, err := f.Read(buf)
		b.Write(buf[:n])
		if err != nil {
			return b.String()
		}
	}
}

type frameBytes struct{ b []byte }

func (f *frameBytes) Write(p []byte) (int, error) { f.b = append(f.b, p...); return len(p), nil }
