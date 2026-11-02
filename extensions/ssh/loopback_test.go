package ssh_test

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/oolong/ui/core/scene"

	charmssh "charm.land/ssh"

	"github.com/Tangerg/oolong/extensions/ssh"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/program"
	"github.com/Tangerg/oolong/ui/core/term"
	gossh "golang.org/x/crypto/ssh"
)

// These run against a real SSH server and a real client, because the thing being
// checked is what the dependency does and not what this package believes it does.
//
// Three facts have to hold at once for an interface to work over a session, and a
// mode that gives two of them is no use: Run must be the only reader of the channel,
// the only consumer of its window changes, and its frames must reach the client as
// the bytes it wrote. Nothing short of an accepted connection demonstrates any of
// them — a stand-in session answers whatever it was told to.

func serveClient(t *testing.T, adapter *ssh.Adapter, run func(*ssh.Adapter, charmssh.Session), options ...charmssh.Option) *gossh.Client {
	t.Helper()
	var listen net.ListenConfig
	listener, err := listen.Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := &charmssh.Server{Handler: func(s charmssh.Session) { run(adapter, s) }}
	for _, option := range options {
		if optionErr := option(server); optionErr != nil {
			t.Fatal(optionErr)
		}
	}
	if installErr := server.SetOption(adapter.Install); installErr != nil {
		t.Fatal(installErr)
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
	return client
}

func serve(t *testing.T, run func(*ssh.Adapter, charmssh.Session)) *gossh.Session {
	t.Helper()
	return requestSession(t, serveClient(t, new(ssh.Adapter), run), 24, 80)
}

func requestSession(t *testing.T, client *gossh.Client, rows, cols int) *gossh.Session {
	t.Helper()
	session, err := client.NewSession()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	if err := session.RequestPty("xterm-256color", rows, cols, gossh.TerminalModes{}); err != nil {
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
	client := serve(t, func(adapter *ssh.Adapter, session charmssh.Session) {
		done <- runSSHFixture(adapter, session, program.ScreenConfig{
			Build: func(runtime *program.Runtime) (scene.Element, error) {
				return fixtureElement(keyReader{runtime: runtime, keys: keys}), nil
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
	client := serve(t, func(adapter *ssh.Adapter, session charmssh.Session) {
		done <- runSSHFixture(adapter, session, program.ScreenConfig{
			Build: func(runtime *program.Runtime) (scene.Element, error) {
				return fixtureElement(&resizeReader{runtime: runtime, sizes: sizes}), nil
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
	const frame = "top\r\nbottom\x1b7\x1b[BX\x1b8"
	cfg := term.Config{AltScreen: true}
	done := make(chan error, 1)
	client := serve(t, func(adapter *ssh.Adapter, session charmssh.Session) {
		terminal, err := adapter.Bind(session)
		if err != nil {
			done <- err
			return
		}
		host, err := terminal.Open(cfg)
		if err != nil {
			done <- err
			return
		}
		_, err = host.Writer().Queue([]byte(frame))
		done <- errors.Join(err, host.Close())
	})
	var output bytes.Buffer
	client.Stdout = &output
	if err := client.Shell(); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if err := client.Wait(); err != nil {
		t.Fatal(err)
	}
	modes := cfg.Modes(nil)
	if got, want := output.String(), modes.Enter()+frame+modes.Leave(); got != want {
		t.Fatalf("client received %q, want %q", got, want)
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
			client := serve(t, func(adapter *ssh.Adapter, session charmssh.Session) {
				done <- runSSHFixture(adapter, session, program.ScreenConfig{
					Terminal: features, Build: func(runtime *program.Runtime) (scene.Element, error) {
						return fixtureElement(&canvas{runtime: runtime, paint: test.paint}), nil
					},
				})
			})
			var output bytes.Buffer
			client.Stdout = &output
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
			if err := client.Wait(); err != nil {
				t.Fatal(err)
			}
			sent := output.String()
			if !test.refused && (!strings.Contains(sent, "top") || !strings.Contains(sent, "bottom") || !strings.Contains(sent, "\x1b7\x1b[BX\x1b8")) {
				t.Fatalf("client did not receive the composed frame: %q", sent)
			}
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

func TestAdapterRefusesAnOSAllocatorBeforeServing(t *testing.T) {
	server := new(charmssh.Server)
	if err := server.SetOption(charmssh.AllocatePty()); err != nil {
		t.Fatal(err)
	}
	var adapter ssh.Adapter
	if err := server.SetOption(adapter.Install); err == nil {
		t.Fatal("emulation took over an OS PTY server")
	}
}

func awaitSSH[T any](t *testing.T, result <-chan T) T {
	t.Helper()
	select {
	case value := <-result:
		return value
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for the SSH session")
		var zero T
		return zero
	}
}

// An acknowledgement on this channel follows every earlier window-change on it.
func synchronizeRequests(t *testing.T, session *gossh.Session) {
	t.Helper()
	done := make(chan error, 1)
	go func() {
		_, err := session.SendRequest("oolong-test-barrier", true, nil)
		done <- err
	}()
	if err := awaitSSH(t, done); err != nil {
		t.Fatal(err)
	}
}

func TestResizesBeforeShellAndDuringHandlerSetup(t *testing.T) {
	for _, phase := range []string{"before shell", "during handler setup"} {
		t.Run(phase, func(t *testing.T) {
			entered := make(chan struct{})
			open := make(chan struct{})
			done := make(chan error, 1)
			sizes := make(chan [2]int, 1)
			client := serve(t, func(adapter *ssh.Adapter, session charmssh.Session) {
				close(entered)
				select {
				case <-open:
				case <-session.Context().Done():
					return
				}
				done <- runSSHFixture(adapter, session, program.ScreenConfig{
					Build: func(rt *program.Runtime) (scene.Element, error) {
						return fixtureElement(&resizeReader{runtime: rt, sizes: sizes}), nil
					},
				})
			})
			if _, err := client.StdinPipe(); err != nil {
				t.Fatal(err)
			}
			if phase == "during handler setup" {
				if err := client.Shell(); err != nil {
					t.Fatal(err)
				}
				awaitSSH(t, entered)
			}
			for _, columns := range []int{81, 95, 100} {
				if err := client.WindowChange(30, columns); err != nil {
					t.Fatal(err)
				}
			}
			synchronizeRequests(t, client)
			if phase == "before shell" {
				if err := client.Shell(); err != nil {
					t.Fatal(err)
				}
			}
			close(open)
			if got := awaitSSH(t, sizes); got != [2]int{100, 30} {
				t.Fatalf("painted size = %v", got)
			}
			if err := awaitSSH(t, done); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestSessionClaimsAreIndependentOnOneConnection(t *testing.T) {
	done := make(chan error, 2)
	client := serveClient(t, new(ssh.Adapter), func(adapter *ssh.Adapter, session charmssh.Session) {
		terminal, err := adapter.Bind(session)
		if err != nil {
			done <- err
			return
		}
		host, err := terminal.Open(term.Config{})
		if err != nil {
			done <- err
			return
		}
		w, h, sizeErr := host.Size()
		_, writeErr := host.Writer().Queue(fmt.Appendf(nil, "window=%dx%d", w, h))
		var claimErr error
		copied := *terminal
		if second, openErr := copied.Open(term.Config{}); openErr == nil {
			claimErr = errors.Join(errors.New("opened the same terminal twice"), second.Close())
		}
		done <- errors.Join(sizeErr, writeErr, claimErr, host.Close())
	})
	// A channel with a granted PTY may end without ever starting a shell.
	dropped := requestSession(t, client, 15, 70)
	if err := dropped.Close(); err != nil {
		t.Fatal(err)
	}
	first := requestSession(t, client, 20, 82)
	second := requestSession(t, client, 30, 103)
	var outputs [2]bytes.Buffer
	for i, session := range []*gossh.Session{first, second} {
		session.Stdout = &outputs[i]
		if err := session.Shell(); err != nil {
			t.Fatal(err)
		}
	}
	for i, session := range []*gossh.Session{first, second} {
		if err := awaitSSH(t, done); err != nil {
			t.Fatal(err)
		}
		if err := session.Wait(); err != nil {
			t.Fatal(err)
		}
		want := []string{"window=82x20", "window=103x30"}[i]
		if got := outputs[i].String(); !strings.Contains(got, want) {
			t.Fatalf("session %d got %q, want %s", i, got, want)
		}
	}
}

func TestInvalidPreShellResizeStillDrainsTheSSHRequestLoop(t *testing.T) {
	done := make(chan error, 1)
	client := serve(t, func(adapter *ssh.Adapter, session charmssh.Session) {
		terminal, err := adapter.Bind(session)
		if err == nil {
			host, openErr := terminal.Open(term.Config{})
			err = openErr
			if err == nil {
				source := host.Input()
				for range source.Events() {
				}
				err = errors.Join(source.Err(), host.Close())
			}
		}
		done <- err
	})
	if _, err := client.StdinPipe(); err != nil {
		t.Fatal(err)
	}
	if err := client.WindowChange(2, grid.MaxTerminalCells); err != nil {
		t.Fatal(err)
	}
	for range 4 {
		if err := client.WindowChange(24, 90); err != nil {
			t.Fatal(err)
		}
	}
	synchronizeRequests(t, client)
	if err := client.Shell(); err != nil {
		t.Fatal(err)
	}
	if err := awaitSSH(t, done); !errors.Is(err, ssh.ErrWindowSize) {
		t.Fatalf("invalid resize outcome = %v", err)
	}
}

func TestRejectedPTYLeavesTheChannelAvailable(t *testing.T) {
	done := make(chan error, 1)
	client := serveClient(t, new(ssh.Adapter), func(adapter *ssh.Adapter, session charmssh.Session) {
		_, err := adapter.Bind(session)
		done <- err
	}, func(server *charmssh.Server) error {
		server.PtyCallback = func(_ charmssh.Context, pty charmssh.Pty) bool { return pty.Term == "xterm" }
		return nil
	})
	for _, retry := range []bool{false, true} {
		session, err := client.NewSession()
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = session.Close() })
		if err := session.RequestPty("denied", 24, 80, nil); err == nil {
			t.Fatal("a request denied by the PTY policy was accepted")
		}
		if err := session.WindowChange(25, 81); err != nil {
			t.Fatal(err)
		}
		synchronizeRequests(t, session)
		want := ssh.ErrNoPTY
		if retry {
			if err := session.RequestPty("xterm", 24, 80, nil); err != nil {
				t.Fatal(err)
			}
			want = nil
		}
		if err := session.Shell(); err != nil {
			t.Fatal(err)
		}
		if err := awaitSSH(t, done); !errors.Is(err, want) {
			t.Fatalf("retry %t: Bind = %v, want %v", retry, err, want)
		}
		if err := session.Wait(); err != nil {
			t.Fatalf("retry %t: channel exit = %v", retry, err)
		}
	}
}
