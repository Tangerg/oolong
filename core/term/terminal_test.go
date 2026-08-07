package term_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/term"
)

// A terminal is the one thing here that cannot be stood in for, so these open a
// real one. What is asserted is the lifecycle: that taking it over says what it
// means to, that it comes back the way it was found, and that the parts a caller
// touches work on a terminal that is not this process's own.

// pty opens a pty pair and closes both ends when the test is done, skipping
// where there is no pty to open.
//
// The opening itself lives in the platform files, which is where the syscalls
// are: a test file that named one could not be compiled on a platform without
// it, and this package is meant to build everywhere it claims to.
func pty(t *testing.T) (primary, replica *os.File) {
	t.Helper()
	primary, replica, err := openPTY()
	if err != nil {
		t.Skipf("no pty here: %v", err)
	}
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

func TestOpeningRefusesRedirectedOutputEvenWithTerminalInput(t *testing.T) {
	_, replica := pty(t)
	out, err := os.CreateTemp(t.TempDir(), "redirected-output")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = out.Close() }()

	if _, err := term.OpenOn(replica, out, term.Options{}); !errors.Is(err, term.ErrNotTerminal) {
		t.Fatalf("OpenOn error = %v, want redirected output rejected", err)
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

func TestCloseStillAttemptsRawModeRestoreAfterItsOutputFails(t *testing.T) {
	primary, replica := pty(t)
	tty, err := term.OpenOn(replica, replica, term.Options{Mouse: true})
	if err != nil {
		t.Fatalf("opening a pty as a terminal: %v", err)
	}
	read(t, primary, 300*time.Millisecond)

	// Closing the underlying terminal injects failures into both independent teardown
	// steps. Close must attempt both and join their errors; stopping after the failed
	// leave sequence would strand a still-valid terminal in raw mode in the analogous
	// real failure.
	if closeErr := replica.Close(); closeErr != nil {
		t.Fatal(closeErr)
	}
	err = tty.Close()
	if err == nil {
		t.Fatal("Close hid its teardown failures")
	}
	for _, step := range []string{"give the terminal back", "leave raw mode"} {
		if !strings.Contains(err.Error(), step) {
			t.Errorf("Close error %q does not include %q", err, step)
		}
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

func TestATerminalReportsALaterResize(t *testing.T) {
	_, replica := pty(t)
	tty, err := term.OpenOn(replica, replica, term.Options{})
	if err != nil {
		t.Fatalf("opening a pty as a terminal: %v", err)
	}
	t.Cleanup(func() { _ = tty.Close() })
	<-tty.Events() // opening geometry

	width, height, err := tty.Size()
	if err != nil {
		t.Fatalf("asking the opening size: %v", err)
	}
	wantWidth, wantHeight := width+1, height+1
	if err := resizePTY(replica, wantWidth, wantHeight); err != nil {
		t.Skipf("no later resize source here: %v", err)
	}

	select {
	case event := <-tty.Events():
		resized, ok := event.(input.Resize)
		if !ok {
			t.Fatalf("event after resize = %#v, want input.Resize", event)
		}
		if resized.Width != wantWidth || resized.Height != wantHeight {
			t.Fatalf("resize = %dx%d, want %dx%d", resized.Width, resized.Height, wantWidth, wantHeight)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("a later terminal resize never reached the event stream")
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
	if err := tty.Writer().Drain(2 * time.Second); err != nil {
		t.Fatalf("the frame never reached the terminal: %v", err)
	}
	if seen := read(t, watch, 500*time.Millisecond); !strings.Contains(seen, "hi") {
		t.Fatalf("the terminal was sent %q, want the frame in it", seen)
	}
}

func TestKeystrokesTypedAtATerminalArriveAsEvents(t *testing.T) {
	tty, watch := open(t, term.Options{})
	<-tty.Events() // the opening size

	if _, err := watch.WriteString("k"); err != nil {
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

func TestATerminalHandedOverIsGivenBackWholeAndTakenBackWhole(t *testing.T) {
	// The session records the modes it turned on and unwinds them in the opposite
	// order, which is what closing does — so handing the terminal to a child is that
	// twice, and the child gets a terminal with no idea a program was using it.
	primary, replica := pty(t)
	tty, err := term.OpenOn(replica, replica, term.Options{AltScreen: true, Mouse: true})
	if err != nil {
		t.Fatalf("opening a pty as a terminal: %v", err)
	}
	defer func() { _ = tty.Close() }()
	<-tty.Events() // the opening size
	read(t, primary, 200*time.Millisecond)

	typed := make(chan string, 1)
	err = tty.Hand(func() error {
		// The sequences are written out here rather than read from the package: what
		// is asserted is what a terminal is sent, and a test that asked the code what
		// it sends would pass whatever it happened to send.
		if seen := read(t, primary, time.Second); !strings.Contains(seen, "\x1b[?1049l") ||
			!strings.Contains(seen, "\x1b[?1003l") {
			t.Errorf("the child was given a terminal still holding %q", seen)
		}

		// And the reader is off the terminal, which is the half nothing else can do
		// for a caller: a session that only put the modes back would still be reading,
		// and every other keystroke would go to this process instead of to the child.
		go func() {
			buf := make([]byte, 8)
			n, _ := replica.Read(buf)
			typed <- string(buf[:n])
		}()
		time.Sleep(20 * time.Millisecond)
		if _, writeErr := primary.WriteString("k\n"); writeErr != nil {
			return writeErr
		}
		select {
		case got := <-typed:
			if got != "k\n" {
				t.Errorf("the child read %q, want the whole keystroke", got)
			}
		case <-time.After(2 * time.Second):
			t.Error("the session's reader took the byte the child was meant to get")
		}
		return nil
	})
	if errors.Is(err, errors.ErrUnsupported) {
		t.Skip("a reader cannot be taken off the terminal here")
	}
	if err != nil {
		t.Fatalf("handing the terminal over: %v", err)
	}

	if seen := read(t, primary, time.Second); !strings.Contains(seen, "\x1b[?1049h") {
		t.Errorf("the terminal was taken back as %q, want the modes on again", seen)
	}
	if _, err := primary.WriteString("z"); err != nil {
		t.Fatal(err)
	}
	deadline := time.After(2 * time.Second)
	for {
		select {
		case ev := <-tty.Events():
			// A fresh size arrives as well, because nothing reported one while the
			// terminal was somebody else's: the signal went to whatever was in the
			// foreground.
			if key, ok := ev.(input.Key); ok {
				if key.Rune != 'z' {
					t.Fatalf("got %#v, want the keystroke", ev)
				}
				return
			}
		case <-deadline:
			t.Fatal("the reader never went back on the terminal")
		}
	}
}

func TestAPanickingChildStillReturnsTerminalOwnership(t *testing.T) {
	primary, replica := pty(t)
	tty, err := term.OpenOn(replica, replica, term.Options{AltScreen: true})
	if err != nil {
		t.Fatalf("opening a pty as a terminal: %v", err)
	}
	defer func() { _ = tty.Close() }()
	<-tty.Events()
	read(t, primary, 200*time.Millisecond)

	panicked := false
	var handErr error
	func() {
		defer func() { panicked = recover() != nil }()
		handErr = tty.Hand(func() error { panic("child failed") })
	}()
	if errors.Is(handErr, errors.ErrUnsupported) {
		t.Skip("a reader cannot be taken off the terminal here")
	}
	if handErr != nil {
		t.Fatalf("handing the terminal over: %v", handErr)
	}
	if !panicked {
		t.Fatal("child panic did not continue through Hand")
	}
	if seen := read(t, primary, time.Second); !strings.Contains(seen, "\x1b[?1049h") {
		t.Fatalf("terminal was not taken back after panic; output %q", seen)
	}

	if _, err := primary.WriteString("k"); err != nil {
		t.Fatal(err)
	}
	deadline := time.After(2 * time.Second)
	for {
		select {
		case event := <-tty.Events():
			if key, ok := event.(input.Key); ok {
				if key.Rune != 'k' {
					t.Fatalf("event after panic = %#v", event)
				}
				return
			}
		case <-deadline:
			t.Fatal("terminal reader remained parked after panic")
		}
	}
}

func TestWhatASessionSaysToTheTerminalBesideItsFrames(t *testing.T) {
	// The three that are one sequence each and are ignored by a terminal that does
	// not implement them, which is what makes them safe to send without asking.
	tty, watch := open(t, term.Options{})
	read(t, watch, 200*time.Millisecond)

	// With an escape byte in the text, because the text is a file name or a model's
	// answer as often as it is a constant. One that survived would end the sequence
	// and leave the rest to be read as commands.
	tty.SetTitle("building \x1b]0;oolong")
	tty.Bell()
	tty.Notify("tests passed")
	if err := tty.Writer().Drain(2 * time.Second); err != nil {
		t.Fatalf("what was said never reached the terminal: %v", err)
	}

	seen := read(t, watch, 500*time.Millisecond)
	if !strings.Contains(seen, "\x1b[22;0t") {
		t.Errorf("the title the terminal had was not kept: %q", seen)
	}
	if !strings.Contains(seen, "\x1b]0;building ]0;oolong\x07") {
		t.Errorf("the title was sent as %q", seen)
	}
	if !strings.Contains(seen, "\x1b]9;tests passed\x07") {
		t.Errorf("the notification was sent as %q", seen)
	}
	if !strings.Contains(seen, "\x07") {
		t.Errorf("the bell was not rung: %q", seen)
	}

	// And put back on the way out, for the same reason a mode is: a shell whose
	// window is still called "building oolong" an hour later is a program that left
	// something behind.
	if err := tty.Close(); err != nil {
		t.Fatalf("closing: %v", err)
	}
	if seen := read(t, watch, 500*time.Millisecond); !strings.Contains(seen, "\x1b[23;0t") {
		t.Errorf("the terminal was given back as %q, want its own title with it", seen)
	}
}

func TestSomewhereToWriteWhileTheTerminalIsTaken(t *testing.T) {
	// A line printed to standard output lands in the middle of a frame and is gone by
	// the next repaint, so a program being debugged writes here instead.
	path := filepath.Join(t.TempDir(), "debug.log")
	for _, line := range []string{"first\n", "second\n"} {
		f, err := term.LogTo(path)
		if err != nil {
			t.Fatalf("opening a log: %v", err)
		}
		if _, err := f.WriteString(line); err != nil {
			t.Fatal(err)
		}
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
	}
	// Appending, so a run does not lose the run before it.
	//nolint:gosec // G304: the path is this test's own temporary directory.
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "first\nsecond\n" {
		t.Fatalf("the log holds %q", got)
	}
}
