package ssh_test

import (
	"errors"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	charmssh "charm.land/ssh"
	gossh "golang.org/x/crypto/ssh"

	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/program"
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

// serve accepts one SSH session and hands it to run, returning the client end.
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

// painter draws one frame of known bytes and stops.
type painter struct {
	runtime *program.Runtime
	drawn   bool
}

func (p *painter) Draw(view grid.View) {
	view.Text(0, 0, "top", grid.Style{FG: grid.RGBColor(0x80, 0, 0)})
	view.Text(0, 1, "bottom", grid.Style{})
	if p.drawn {
		p.runtime.Quit()
	}
	p.drawn = true
}

func (*painter) Handle(input.Event) bool { return false }

func TestAFrameReachesTheClientAsTheBytesItWasWritten(t *testing.T) {
	// The server's default handling emulates the terminal, and the emulation is a
	// writer: it turns a line feed into a carriage return and a line feed. A frame is
	// exact bytes, so the only reason this holds is that no frame contains a line
	// feed that is not already the second half of one — which is a claim about the
	// renderer and is therefore checked against a real client rather than asserted.
	done := make(chan error, 1)
	client := serve(t, func(session charmssh.Session) {
		done <- ssh.Run(session, program.Config{
			Root: func(runtime *program.Runtime) program.Component {
				return &painter{runtime: runtime}
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
	for _, want := range []string{"\x1b[1;1H", "top", "\x1b[2;1H", "bottom"} {
		if !strings.Contains(got, want) {
			t.Fatalf("the client received %q, without %q", got, want)
		}
	}
	// A lone line feed is what the emulation rewrites, and finding one means a frame
	// reached the client as bytes it was not written as.
	for at := range len(got) {
		if got[at] == '\n' && (at == 0 || got[at-1] != '\r') {
			t.Fatalf("the client received a rewritten frame at byte %d: %q", at, got)
		}
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
