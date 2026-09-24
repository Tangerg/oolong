// The contract of [ssh.Run] is checked from outside the package, because that is
// where its callers stand. A test that shares the package can reach an unexported
// helper to arrange a case the published signature cannot express, and then the
// passing test says nothing about whether the operation is usable at all.
package ssh_test

import (
	"context"
	"errors"
	"io"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	charmssh "charm.land/ssh"

	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/program"
	"github.com/Tangerg/oolong/core/programtest"
	"github.com/Tangerg/oolong/core/term"
	"github.com/Tangerg/oolong/ssh"
)

func TestRunOwnsModesButNotTheSSHExit(t *testing.T) {
	windows := make(chan charmssh.Window)
	close(windows)
	session := &fakeSession{
		ctx:     newFakeContext(t.Context()),
		window:  charmssh.Window{Width: 80, Height: 24},
		windows: windows,
		ptyOK:   true,
		environ: []string{"TERM=dumb", "COLORTERM=truecolor"},
	}

	err := ssh.Run(session, program.Config{
		Root: func(runtime *program.Runtime) program.Component {
			return quittingComponent{runtime: runtime}
		},
		Terminal: term.Features{
			Mouse: true, Focus: true, Keyboard: term.KeyboardCompatible,
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	all := term.Config{
		AltScreen: true,
		Features:  term.Features{Mouse: true, Focus: true, Keyboard: term.KeyboardCompatible},
	}.Modes(nil)
	written := session.output.String()
	if !strings.HasPrefix(written, all.Enter()) {
		t.Fatalf("output did not acquire modes first: %q", written)
	}
	if !strings.HasSuffix(written, all.Leave()) {
		t.Fatalf("output did not release modes last: %q", written)
	}
	if session.closed {
		t.Fatal("Run closed the caller-owned SSH channel")
	}
}

func TestRunUsesTheClientEnvironmentForKeyboardCompatibility(t *testing.T) {
	windows := make(chan charmssh.Window)
	close(windows)
	session := &fakeSession{
		ctx:     newFakeContext(t.Context()),
		window:  charmssh.Window{Width: 80, Height: 24},
		windows: windows,
		ptyOK:   true,
		environ: []string{
			"WSL_DISTRO_NAME=Ubuntu",
			"TERM_PROGRAM=vscode",
		},
	}

	err := ssh.Run(session, program.Config{
		Root: func(runtime *program.Runtime) program.Component {
			return quittingComponent{runtime: runtime}
		},
		Terminal: term.Features{Keyboard: input.KeyboardAll},
	})
	if err != nil {
		t.Fatal(err)
	}
	if written := session.output.String(); strings.Contains(written, "\x1b[>") {
		t.Fatalf("keyboard mode ignored the client environment: %q", written)
	}
}

func TestRunUsesTheClientEnvironmentForItsWheel(t *testing.T) {
	windows := make(chan charmssh.Window)
	close(windows)
	session := &fakeSession{
		ctx:     newFakeContext(t.Context()),
		window:  charmssh.Window{Width: 80, Height: 24},
		windows: windows,
		ptyOK:   true,
		environ: []string{"TERM_PROGRAM=iTerm.app"},
	}

	var got input.Wheel
	err := ssh.Run(session, program.Config{
		Root: func(runtime *program.Runtime) program.Component {
			got = runtime.Environment().Wheel()
			return quittingComponent{runtime: runtime}
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := input.Wheel{Reports: 1, Rows: 1, Trackpad: 3}
	if got != want {
		t.Fatalf("wheel = %+v, want the client terminal's %+v", got, want)
	}
}

func TestRunUsesTheClientEnvironmentForItsLocale(t *testing.T) {
	windows := make(chan charmssh.Window)
	close(windows)
	session := &fakeSession{
		ctx:     newFakeContext(t.Context()),
		window:  charmssh.Window{Width: 80, Height: 24},
		windows: windows,
		ptyOK:   true,
		environ: []string{"LC_CTYPE=C", "LANG=en_US.UTF-8"},
	}

	var got string
	err := ssh.Run(session, program.Config{
		Root: func(runtime *program.Runtime) program.Component {
			got = runtime.Environment().Locale()
			return quittingComponent{runtime: runtime}
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != "C" {
		t.Fatalf("locale = %q, want the client terminal's LC_CTYPE", got)
	}
}

func TestRunPreservesAnOutputFailure(t *testing.T) {
	want := errors.New("connection reset")
	windows := make(chan charmssh.Window)
	close(windows)
	session := &fakeSession{
		ctx:       newFakeContext(t.Context()),
		window:    charmssh.Window{Width: 80, Height: 24},
		windows:   windows,
		ptyOK:     true,
		outputErr: want,
	}
	err := ssh.Run(session, program.Config{
		Root: func(runtime *program.Runtime) program.Component {
			return quittingComponent{runtime: runtime}
		},
	})
	if !errors.Is(err, want) {
		t.Fatalf("error = %v, want %v", err, want)
	}
}

func TestRunRejectsAContradictoryTransport(t *testing.T) {
	session := &fakeSession{ptyOK: true, window: charmssh.Window{Width: 80, Height: 24}}
	err := ssh.Run(session, program.Config{
		Host: programtest.New(t, programtest.Config{Width: 80, Height: 24}),
	})
	if !errors.Is(err, ssh.ErrHostSet) {
		t.Fatalf("error = %v, want ErrHostSet", err)
	}
}

func TestRunRefusesATerminalTheServerAllocated(t *testing.T) {
	// The library starts copying the channel into that terminal, and draining its
	// window changes, before the handler runs. Both are Run's, and neither can be
	// shared: the keystroke that went to the other reader is gone.
	session := &fakeSession{
		ctx:       newFakeContext(t.Context()),
		window:    charmssh.Window{Width: 80, Height: 24},
		ptyOK:     true,
		allocated: true,
	}
	err := ssh.Run(session, program.Config{
		Root: func(runtime *program.Runtime) program.Component {
			return quittingComponent{runtime: runtime}
		},
	})
	if !errors.Is(err, ssh.ErrAllocatedPTY) {
		t.Fatalf("error = %v, want ErrAllocatedPTY", err)
	}
	if got := session.output.String(); got != "" {
		t.Fatalf("a session with a competing reader was written to: %q", got)
	}
}

func TestRunValidatesTheProgramBeforeTakingTheSession(t *testing.T) {
	session := &fakeSession{ptyOK: true, window: charmssh.Window{Width: 80, Height: 24}}
	if err := ssh.Run(session, program.Config{}); err == nil {
		t.Fatal("invalid program configuration was accepted")
	}
	if got := session.output.String(); got != "" {
		t.Fatalf("invalid configuration wrote %q", got)
	}
}

func TestRunRequiresAnAllocatedCellWindow(t *testing.T) {
	for _, tc := range []struct {
		name     string
		window   charmssh.Window
		pty      bool
		wantErr  error
		wantSize bool
	}{
		{"no PTY", charmssh.Window{}, false, ssh.ErrNoPTY, false},
		{"no columns", charmssh.Window{Height: 24}, true, ssh.ErrWindowSize, true},
		{"no rows", charmssh.Window{Width: 80}, true, ssh.ErrWindowSize, true},
		{"too many cells", charmssh.Window{Width: program.MaxCells, Height: 2}, true, ssh.ErrWindowSize, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			session := &fakeSession{ptyOK: tc.pty, window: tc.window}
			err := ssh.Run(session, program.Config{Root: func(*program.Runtime) program.Component {
				return quittingComponent{}
			}})
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("error = %v, want %v", err, tc.wantErr)
			}
			if tc.wantSize && !errors.Is(err, program.ErrInvalidSize) {
				t.Fatalf("error = %v, want underlying program.ErrInvalidSize", err)
			}
		})
	}
}

type quittingComponent struct{ runtime *program.Runtime }

func (c quittingComponent) Draw(grid.View)        { c.runtime.Quit() }
func (quittingComponent) Handle(input.Event) bool { return false }

type fakeSession struct {
	charmssh.Session
	ctx     charmssh.Context
	window  charmssh.Window
	windows <-chan charmssh.Window
	ptyOK   bool
	// allocated is the server having given the session a terminal of its own, which
	// is the mode Run refuses. The zero value is the library's default handling.
	allocated bool
	environ   []string
	input     strings.Reader
	output    lockedBuffer
	outputErr error
	closed    bool
}

func (s *fakeSession) Context() charmssh.Context { return s.ctx }
func (s *fakeSession) Environ() []string         { return append([]string(nil), s.environ...) }
func (s *fakeSession) Pty() (charmssh.Pty, <-chan charmssh.Window, bool) {
	return charmssh.Pty{Term: "xterm-256color", Window: s.window}, s.windows, s.ptyOK
}
func (s *fakeSession) EmulatedPty() bool          { return !s.allocated }
func (s *fakeSession) Read(p []byte) (int, error) { return s.input.Read(p) }
func (s *fakeSession) Write(p []byte) (int, error) {
	if s.outputErr != nil {
		return 0, s.outputErr
	}
	return s.output.Write(p)
}

func (s *fakeSession) Close() error {
	s.closed = true
	return nil
}

type lockedBuffer struct {
	sync.Mutex
	b strings.Builder
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.Lock()
	defer b.Unlock()
	return b.b.Write(p)
}

func (b *lockedBuffer) String() string {
	b.Lock()
	defer b.Unlock()
	return b.b.String()
}

type fakeContext struct {
	sync.Mutex
	done        <-chan struct{}
	err         func() error
	permissions charmssh.Permissions
}

func newFakeContext(ctx context.Context) *fakeContext {
	return &fakeContext{done: ctx.Done(), err: ctx.Err}
}

func (*fakeContext) Deadline() (time.Time, bool)          { return time.Time{}, false }
func (c *fakeContext) Done() <-chan struct{}              { return c.done }
func (c *fakeContext) Err() error                         { return c.err() }
func (*fakeContext) Value(any) any                        { return nil }
func (*fakeContext) User() string                         { return "test" }
func (*fakeContext) SessionID() string                    { return "session" }
func (*fakeContext) ClientVersion() string                { return "client" }
func (*fakeContext) ServerVersion() string                { return "server" }
func (*fakeContext) RemoteAddr() net.Addr                 { return nil }
func (*fakeContext) LocalAddr() net.Addr                  { return nil }
func (c *fakeContext) Permissions() *charmssh.Permissions { return &c.permissions }
func (*fakeContext) SetValue(any, any)                    {}

var _ io.Writer = (*lockedBuffer)(nil)
