package ssh_test

import (
	"errors"
	"image"
	"io"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	charmssh "charm.land/ssh"
	gossh "golang.org/x/crypto/ssh"

	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/program"
	"github.com/Tangerg/oolong/core/term"
	"github.com/Tangerg/oolong/ssh"
)

// These run against a real SSH server and a real client, because the thing being
// checked is what the dependency does and not what this package believes it does.
//
// Three facts have to hold at once for an interface to work over a session, and a
// mode that gives two of them is no use: Run must be the only reader of the channel,
// the only consumer of its window changes, and its frames must reach the client as
// the bytes it wrote. Nothing short of an accepted connection demonstrates any of
// them — a stand-in session answers whatever it was told to.

// recorded is a session that keeps what was written through it, and is otherwise
// the session it was given. It stands in for nothing: every call reaches the real
// session, and what it records is the argument the transport was handed.
type recorded struct {
	charmssh.Session
	mu   sync.Mutex
	sent []byte
}

func (r *recorded) Write(p []byte) (int, error) {
	r.mu.Lock()
	r.sent = append(r.sent, p...)
	r.mu.Unlock()
	return r.Session.Write(p)
}

func (r *recorded) written() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return string(r.sent)
}

func serve(t *testing.T, run func(charmssh.Session), options ...charmssh.Option) *gossh.Session {
	t.Helper()
	var listen net.ListenConfig
	listener, err := listen.Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := &charmssh.Server{Handler: run}
	for _, option := range options {
		if optionErr := option(server); optionErr != nil {
			t.Fatal(optionErr)
		}
	}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { _ = server.Close() })

	client, err := gossh.Dial("tcp", listener.Addr().String(), &gossh.ClientConfig{
		User: "oolong",
		// The server generates a key of its own on the first connection and the
		// test has no way to know it in advance. What is being connected to is a
		// listener this test just opened on the loopback interface.
		HostKeyCallback: gossh.InsecureIgnoreHostKey(), //nolint:gosec // loopback test server
		Timeout:         10 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	session, err := client.NewSession()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	if err := session.RequestPty("xterm-256color", 24, 80, gossh.TerminalModes{}); err != nil {
		t.Fatal(err)
	}
	return session
}

// keyReader quits on the first key and records what it was.
type keyReader struct {
	runtime *program.Runtime
	keys    chan input.Key
}

func (keyReader) Draw(grid.View) {}

func (r keyReader) Handle(event input.Event) bool {
	key, ok := event.(input.Key)
	if !ok {
		return false
	}
	select {
	case r.keys <- key:
	default:
	}
	r.runtime.Quit()
	return true
}

func TestAKeystrokeReachesTheProgramOverARealSession(t *testing.T) {
	keys := make(chan input.Key, 1)
	done := make(chan error, 1)
	client := serve(t, func(session charmssh.Session) {
		done <- ssh.Run(session, program.Config{
			Root: func(runtime *program.Runtime) program.Component {
				return keyReader{runtime: runtime, keys: keys}
			},
		})
	})
	stdin, err := client.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Shell(); err != nil {
		t.Fatal(err)
	}
	if _, err := stdin.Write([]byte("z")); err != nil {
		t.Fatal(err)
	}

	select {
	case key := <-keys:
		if key.Rune != 'z' {
			t.Fatalf("the program was given %+v, want the z that was typed", key)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the keystroke never reached the program: something else read the channel")
	}
	if err := <-done; err != nil {
		t.Fatalf("Run: %v", err)
	}
}

// resizeReader quits once it is drawn into a surface of the size the client asked
// for. A window change is the program's own event and is never offered to a
// component, so what it did is read off the geometry everything is drawn against.
type resizeReader struct {
	runtime *program.Runtime
	sizes   chan [2]int
}

func (r *resizeReader) Draw(view grid.View) {
	w, h := view.Size()
	if w != 100 || h != 30 {
		return
	}
	select {
	case r.sizes <- [2]int{w, h}:
	default:
	}
	r.runtime.Quit()
}

func (*resizeReader) Handle(input.Event) bool { return false }

func TestAWindowChangeReachesTheProgramOverARealSession(t *testing.T) {
	sizes := make(chan [2]int, 1)
	done := make(chan error, 1)
	client := serve(t, func(session charmssh.Session) {
		done <- ssh.Run(session, program.Config{
			Root: func(runtime *program.Runtime) program.Component {
				return &resizeReader{runtime: runtime, sizes: sizes}
			},
		})
	})
	// The input pipe is held open for the length of the test: a client session with
	// no stdin of its own closes its write side as soon as it starts, and a closed
	// channel takes no further requests.
	if _, err := client.StdinPipe(); err != nil {
		t.Fatal(err)
	}
	out, err := client.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Shell(); err != nil {
		t.Fatal(err)
	}
	// The first frame is what says the program is running and has already asked the
	// session about its terminal. Asking before that answers nothing — and the
	// library writes the new window into the same value the handler reads there.
	if _, err := out.Read(make([]byte, 1)); err != nil {
		t.Fatal(err)
	}
	go func() {
		_, _ = io.Copy(io.Discard, out)
	}()
	if err := client.WindowChange(30, 100); err != nil {
		t.Fatal(err)
	}

	select {
	case size := <-sizes:
		if size != [2]int{100, 30} {
			t.Fatalf("the program drew into %v, want the window the client asked for", size)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the window change never reached the program: something else consumed it")
	}
	if err := <-done; err != nil {
		t.Fatalf("Run: %v", err)
	}
}

// canvas draws one frame and stops, optionally with something painting a region of
// it in bytes this library did not compose.
type canvas struct {
	runtime *program.Runtime
	paint   grid.Painter
	drawn   bool
}

func (c *canvas) Draw(view grid.View) {
	view.Text(0, 0, "top", grid.Style{FG: grid.RGBColor(0x80, 0, 0)})
	view.Text(0, 1, "bottom", grid.Style{})
	if c.paint != nil {
		view.Paint(grid.Area(5, 2, 2, 2), 1, c.paint)
	}
	if c.drawn {
		c.runtime.Quit()
	}
	c.drawn = true
}

func (*canvas) Handle(input.Event) bool { return false }

// lineFeedPainter keeps the one rule a painter has — it leaves the cursor where it
// found it — and writes a line feed of its own to get to the row below.
type lineFeedPainter struct{}

func (lineFeedPainter) Paint(w io.Writer, _ image.Point) error {
	_, err := io.WriteString(w, "\x1b7\nX\x1b8")
	return err
}

func (lineFeedPainter) Erase(io.Writer) error { return nil }

// movingPainter does the same thing the session can carry: it goes down a row by
// asking the terminal to, rather than by writing the byte that means it.
type movingPainter struct{}

func (movingPainter) Paint(w io.Writer, _ image.Point) error {
	_, err := io.WriteString(w, "\x1b7\x1b[BX\x1b8")
	return err
}

func (movingPainter) Erase(io.Writer) error { return nil }

func TestAFrameReachesTheClientAsTheBytesItWasWritten(t *testing.T) {
	// The server's default handling emulates the terminal, and the emulation is a
	// writer: it turns a line feed into a carriage return and a line feed. A frame is
	// exact bytes, so what is compared is what the transport was handed against what
	// the client received — and not the shape of what arrived, which a rewritten
	// frame satisfies just as well as an untouched one.
	done := make(chan error, 1)
	var session *recorded
	client := serve(t, func(accepted charmssh.Session) {
		session = &recorded{Session: accepted}
		done <- ssh.Run(session, program.Config{
			Root: func(runtime *program.Runtime) program.Component {
				return &canvas{runtime: runtime, paint: movingPainter{}}
			},
		})
	})
	out, err := client.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Shell(); err != nil {
		t.Fatal(err)
	}
	received := make(chan string, 1)
	go func() {
		all, _ := io.ReadAll(out)
		received <- string(all)
	}()

	if err := <-done; err != nil {
		t.Fatalf("Run: %v", err)
	}
	var got string
	select {
	case got = <-received:
	case <-time.After(10 * time.Second):
		t.Fatal("the client never saw the session end")
	}
	if sent := session.written(); got != sent {
		t.Fatalf("the client received %d bytes and the session was handed %d:\n got %q\nsent %q",
			len(got), len(sent), got, sent)
	}
	for _, want := range []string{"top", "bottom", "\x1b7\x1b[BX\x1b8"} {
		if !strings.Contains(got, want) {
			t.Fatalf("the client received %q, without %q", got, want)
		}
	}
}

func TestAFrameTheSessionCannotCarryIsRefusedRatherThanRewritten(t *testing.T) {
	// A painter writes bytes this library did not compose, and is free to write a
	// line feed of its own. The emulation would add a carriage return to it and move
	// what the painter drew next to the first column — so the session says it cannot
	// carry that byte instead of carrying something else.
	//
	// Refusing a frame is not the session breaking. Run turned the client's terminal
	// on for the duration of the call and nothing else will turn it off, so the
	// modes it entered have to be left on the way out of a refusal exactly as they
	// are on the way out of a normal exit.
	features := term.Features{Mouse: true, Focus: true, Keyboard: term.KeyboardCompatible}
	modes := term.Config{AltScreen: true, Features: features}.Modes(nil)

	for _, test := range []struct {
		name    string
		paint   grid.Painter
		refused bool
	}{
		{name: "a line feed of its own", paint: lineFeedPainter{}, refused: true},
		{name: "the same move asked for", paint: movingPainter{}},
	} {
		t.Run(test.name, func(t *testing.T) {
			done := make(chan error, 1)
			var session *recorded
			client := serve(t, func(accepted charmssh.Session) {
				session = &recorded{Session: accepted}
				done <- ssh.Run(session, program.Config{
					Root: func(runtime *program.Runtime) program.Component {
						return &canvas{runtime: runtime, paint: test.paint}
					},
					Terminal: features,
				})
			})
			if err := client.Shell(); err != nil {
				t.Fatal(err)
			}

			select {
			case err := <-done:
				switch {
				case test.refused && !errors.Is(err, ssh.ErrLineFeed):
					t.Fatalf("Run = %v, want the frame refused with ErrLineFeed", err)
				case !test.refused && err != nil:
					t.Fatalf("Run = %v, want the frame carried", err)
				}
			case <-time.After(10 * time.Second):
				t.Fatal("the frame was neither sent nor refused")
			}
			sent := session.written()
			if test.refused && strings.Contains(sent, "\x1b7") {
				t.Fatalf("the refused frame was handed to the session anyway: %q", sent)
			}
			if !strings.HasPrefix(sent, modes.Enter()) {
				t.Fatalf("the session never entered the modes Run asked for: %q", sent)
			}
			if !strings.HasSuffix(sent, modes.Leave()) {
				t.Fatalf("the modes Run entered were left on: %q", sent)
			}
		})
	}
}

func TestRunRefusesAServerThatAllocatedTheTerminal(t *testing.T) {
	// charm.land/ssh's AllocatePty starts copying the channel into that terminal, and
	// draining its window changes, before the handler runs. Refusing is the only
	// honest answer: the keystroke that went to the other reader is gone.
	done := make(chan error, 1)
	client := serve(t, func(session charmssh.Session) {
		done <- ssh.Run(session, program.Config{
			Root: func(runtime *program.Runtime) program.Component {
				return keyReader{runtime: runtime, keys: make(chan input.Key, 1)}
			},
		})
	}, charmssh.AllocatePty())
	if err := client.Shell(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if !errors.Is(err, ssh.ErrAllocatedPTY) {
			t.Fatalf("Run = %v, want ErrAllocatedPTY", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Run neither refused the session nor returned")
	}
}
