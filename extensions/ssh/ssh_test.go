// The contract of [ssh.Adapter] is checked from outside the package, because that is
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

	"github.com/Tangerg/oolong/ui/core/scene"

	charmssh "charm.land/ssh"
	"github.com/Tangerg/oolong/extensions/ssh"
	"github.com/Tangerg/oolong/ui/core/clipboard"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/program"
	"github.com/Tangerg/oolong/ui/core/term"
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

	err := runFakeSSHFixture(session, program.ScreenConfig{
		Terminal: term.Features{
			Mouse: true, Focus: true, Keyboard: term.KeyboardCompatible,
		}, Build: func(runtime *program.Runtime) (scene.Element, error) {
			return fixtureElement(quittingComponent{runtime: runtime}), nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	all, err := term.Config{
		AltScreen: true,
		Features:  term.Features{Mouse: true, Focus: true, Keyboard: term.KeyboardCompatible},
	}.Modes(nil)
	if err != nil {
		t.Fatal(err)
	}
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

	err := runFakeSSHFixture(session, program.ScreenConfig{
		Terminal: term.Features{Keyboard: input.KeyboardAll}, Build: func(runtime *program.Runtime) (scene.Element, error) {
			return fixtureElement(quittingComponent{runtime: runtime}), nil
		},
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
	err := runFakeSSHFixture(session, program.ScreenConfig{
		Build: func(runtime *program.Runtime) (scene.Element, error) {
			got = runtime.Environment().Wheel()
			return fixtureElement(quittingComponent{runtime: runtime}), nil
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
	err := runFakeSSHFixture(session, program.ScreenConfig{
		Build: func(runtime *program.Runtime) (scene.Element, error) {
			got = runtime.Environment().Locale()
			return fixtureElement(quittingComponent{runtime: runtime}), nil
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
	err := runFakeSSHFixture(session, program.ScreenConfig{
		Build: func(runtime *program.Runtime) (scene.Element, error) {
			return fixtureElement(quittingComponent{runtime: runtime}), nil
		},
	})
	if !errors.Is(err, want) {
		t.Fatalf("error = %v, want %v", err, want)
	}
}

func TestOpenRequiresASession(t *testing.T) {
	var adapter ssh.Adapter
	if err := adapter.Install(nil); err == nil {
		t.Fatal("missing server accepted")
	}
	if _, err := adapter.Bind(&fakeSession{}); err == nil || errors.Is(err, ssh.ErrNoPTY) {
		t.Fatalf("uninstalled adapter reported a client result: %v", err)
	}
	if _, err := adapter.Bind(nil); err == nil {
		t.Fatal("missing session accepted")
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
	err := runFakeSSHFixture(session, program.ScreenConfig{
		Build: func(runtime *program.Runtime) (scene.Element, error) {
			return fixtureElement(quittingComponent{runtime: runtime}), nil
		},
	})
	if !errors.Is(err, ssh.ErrAllocatedPTY) {
		t.Fatalf("error = %v, want ErrAllocatedPTY", err)
	}
	if got := session.output.String(); got != "" {
		t.Fatalf("a session with a competing reader was written to: %q", got)
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
		{"too many cells", charmssh.Window{Width: grid.MaxTerminalCells, Height: 2}, true, ssh.ErrWindowSize, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			session := &fakeSession{ctx: newFakeContext(t.Context()), ptyOK: tc.pty, window: tc.window}
			err := runFakeSSHFixture(session, program.ScreenConfig{Build: func(*program.Runtime) (scene.Element, error) {
				return fixtureElement(quittingComponent{}), nil
			}})
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("error = %v, want %v", err, tc.wantErr)
			}
			if tc.wantSize && !errors.Is(err, grid.ErrInvalidTerminalSize) {
				t.Fatalf("error = %v, want underlying grid.ErrInvalidTerminalSize", err)
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
func (s *fakeSession) requestedPTY() (charmssh.Pty, bool) {
	return charmssh.Pty{Term: "xterm-256color", Window: s.window}, s.ptyOK
}

func (s *fakeSession) Pty() (charmssh.Pty, <-chan charmssh.Window, bool) {
	pty, ok := s.requestedPTY()
	return pty, s.windows, ok
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

func TestBindRejectsANonComparableSessionWrapper(t *testing.T) {
	var adapter ssh.Adapter
	wrapped := struct {
		charmssh.Session
		tags []string
	}{Session: &fakeSession{}}
	if _, err := adapter.Bind(wrapped); err == nil {
		t.Fatal("a wrapper without a comparable channel identity was accepted")
	}
}

func TestServerAdmitsOneWindowOwner(t *testing.T) {
	server := new(charmssh.Server)
	start := make(chan struct{})
	installed := make(chan error, 2)
	for range cap(installed) {
		go func() {
			var adapter ssh.Adapter
			<-start
			installed <- server.SetOption(adapter.Install)
		}()
	}
	close(start)
	accepted := 0
	for range cap(installed) {
		if <-installed == nil {
			accepted++
		}
	}
	if accepted != 1 {
		t.Fatalf("one server accepted %d window owners", accepted)
	}
}

func TestInstallPreservesAnOccupiedPTYSlot(t *testing.T) {
	called := false
	server := &charmssh.Server{PtyHandler: func(charmssh.Context, charmssh.Session, charmssh.Pty) (func() error, error) {
		called = true
		return func() error { return nil }, nil
	}}
	var adapter ssh.Adapter
	if err := adapter.Install(server); err == nil {
		t.Fatal("an already occupied PTY slot was taken over")
	}
	if _, err := server.PtyHandler(nil, nil, charmssh.Pty{}); err != nil || !called {
		t.Fatalf("existing PTY handler was replaced: called=%t err=%v", called, err)
	}
}

func TestClipboardPassthroughIsExplicitForTheAcceptedRemoteTerminal(t *testing.T) {
	for _, passthrough := range []bool{false, true} {
		windows := make(chan charmssh.Window)
		close(windows)
		session := &fakeSession{
			ctx: newFakeContext(t.Context()), window: charmssh.Window{Width: 80, Height: 24},
			windows: windows, ptyOK: true, environ: []string{"TERM=tmux-256color", "TMUX=/tmp/client-tmux,1,0"},
		}
		err := runFakeSSHFixture(session, program.ScreenConfig{
			Terminal: term.Features{Clipboard: clipboard.Config{TmuxPassthrough: passthrough}},
			Build: func(runtime *program.Runtime) (scene.Element, error) {
				if !runtime.Clipboard().Copy("copied") {
					t.Error("remote clipboard copy refused")
				}
				return fixtureElement(quittingComponent{runtime: runtime}), nil
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		output := session.output.String()
		if wrapped := strings.Contains(output, "\x1bPtmux;"); wrapped != passthrough {
			t.Fatalf("remote environment overrode explicit clipboard transport: %q", output)
		}
		if !passthrough && !strings.Contains(output, "\x1b]52;c;Y29waWVk\x1b\\") {
			t.Fatalf("native remote OSC 52 missing: %q", output)
		}
	}
}

func TestKeyboardRequestObservesRemoteDialectWithoutStartupProbe(t *testing.T) {
	windows := make(chan charmssh.Window)
	close(windows)
	session := &fakeSession{
		ctx: newFakeContext(t.Context()), window: charmssh.Window{Width: 80, Height: 24},
		windows: windows, ptyOK: true,
	}
	session.input.Reset("\x1b[?0u\x1b[1;9P")
	server := new(charmssh.Server)
	var adapter ssh.Adapter
	if err := adapter.Install(server); err != nil {
		t.Fatal(err)
	}
	request, _ := session.requestedPTY()
	release, err := server.PtyHandler(session.ctx, session, request)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if releaseErr := release(); releaseErr != nil {
			t.Error(releaseErr)
		}
	}()
	terminal, err := adapter.Bind(session)
	if err != nil {
		t.Fatal(err)
	}
	host, err := terminal.Open(term.Config{Features: term.Features{Keyboard: term.KeyboardCompatible}})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if closeErr := host.Close(); closeErr != nil {
			t.Error(closeErr)
		}
	}()
	var keys []input.Key
	for event := range host.Input().Events() {
		if key, ok := event.(input.Key); ok {
			keys = append(keys, key)
		}
	}
	if len(keys) != 1 || keys[0].Code != input.F1 || keys[0].Mods != input.Super {
		t.Fatalf("remote zero-enhancement Kitty reply was not observed: %+v", keys)
	}
	if err := host.Writer().Drain(time.Second); err != nil {
		t.Fatal(err)
	}
	if output := session.output.String(); strings.Count(output, "\x1b[?u") != 1 {
		t.Fatalf("remote keyboard request omitted or duplicated its observation: %q", output)
	}
}
