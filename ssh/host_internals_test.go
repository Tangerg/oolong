package ssh

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

	"github.com/Tangerg/oolong/core/clipboard"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/program"
	"github.com/Tangerg/oolong/core/term"
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

	err := Run(session, program.Config{
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

	err := Run(session, program.Config{
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
	err := Run(session, program.Config{
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
	err := Run(session, program.Config{
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

func TestHostClipboardTargetsTheClientTerminal(t *testing.T) {
	var output lockedBuffer
	host := &host{
		writer: term.NewWriter(&output),
		clip:   &clipboard.Channel{},
	}
	if !host.Copy("copied remotely") {
		t.Fatal("a small copy was refused")
	}
	if !host.Paste() {
		t.Fatal("the first paste request was refused")
	}
	if host.Paste() {
		t.Fatal("a second unidentified paste request was accepted")
	}
	if err := host.writer.Close(); err != nil {
		t.Fatal(err)
	}
	written := output.String()
	wantCopy, _ := (&clipboard.Channel{}).Copy(clipboard.System, "copied remotely")
	wantPaste, _ := (&clipboard.Channel{}).Request(clipboard.System)
	for name, sequence := range map[string]string{"copy": wantCopy, "paste": wantPaste} {
		if !strings.Contains(written, sequence) {
			t.Errorf("client %s sequence was not written: %q", name, written)
		}
	}
}

func TestEventSourceTurnsOnlyTheRequestedClipboardAnswerIntoPaste(t *testing.T) {
	reader := make(chanReader)
	windows := make(chan charmssh.Window)
	clip := &clipboard.Channel{}
	if _, ok := clip.Request(clipboard.System); !ok {
		t.Fatal("clipboard request was refused")
	}
	source := newEventSource(t.Context().Done(), reader, windows,
		func(window charmssh.Window) (input.Resize, bool, error) {
			return input.Resize{Width: window.Width, Height: window.Height}, true, nil
		}, clip)
	t.Cleanup(source.Close)

	answer, _ := (&clipboard.Channel{}).Copy(clipboard.System, "from the client")
	reader <- readResult{data: []byte(answer)}
	event := receiveEvent(t, source.Events())
	paste, ok := event.(input.Paste)
	if !ok || paste.Text != "from the client" {
		t.Fatalf("clipboard answer = %#v, want client paste", event)
	}

	unasked, _ := (&clipboard.Channel{}).Copy(clipboard.System, "unasked")
	reader <- readResult{data: []byte(unasked)}
	if event := receiveEvent(t, source.Events()); event == nil {
		t.Fatal("unasked clipboard answer disappeared")
	} else if _, ok := event.(input.OSC); !ok {
		t.Fatalf("unasked clipboard answer became %#v", event)
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
	err := Run(session, program.Config{
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
	err := Run(session, program.Config{Host: (*host)(nil)})
	if !errors.Is(err, ErrHostSet) {
		t.Fatalf("error = %v, want ErrHostSet", err)
	}
}

func TestRunValidatesTheProgramBeforeTakingTheSession(t *testing.T) {
	session := &fakeSession{ptyOK: true, window: charmssh.Window{Width: 80, Height: 24}}
	if err := Run(session, program.Config{}); err == nil {
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
		{"no PTY", charmssh.Window{}, false, ErrNoPTY, false},
		{"no columns", charmssh.Window{Height: 24}, true, ErrWindowSize, true},
		{"no rows", charmssh.Window{Width: 80}, true, ErrWindowSize, true},
		{"too many cells", charmssh.Window{Width: program.MaxCells, Height: 2}, true, ErrWindowSize, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			session := &fakeSession{ptyOK: tc.pty, window: tc.window}
			err := Run(session, program.Config{Root: func(*program.Runtime) program.Component {
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

func TestSessionEnvironmentOwnsTheLastWellFormedValue(t *testing.T) {
	env := newEnvironment([]string{"TERM=dumb", "BROKEN", "=unnamed", "TERM=xterm-256color"})
	if got, ok := env.lookup("TERM"); !ok || got != "xterm-256color" {
		t.Fatalf("TERM = %q, %t", got, ok)
	}
	if _, ok := env.lookup("BROKEN"); ok {
		t.Fatal("malformed entry became an environment value")
	}
}

func TestAWindowUpdateIsAtomicAndZeroKeepsItsAxis(t *testing.T) {
	h := &host{window: charmssh.Window{Width: 80, Height: 24}}
	resized, changed, err := h.resize(charmssh.Window{Height: 40})
	if err != nil {
		t.Fatal(err)
	}
	if !changed || resized != (input.Resize{Width: 80, Height: 40}) {
		t.Fatalf("resize = %#v, %t", resized, changed)
	}
	width, height, err := h.Size()
	if err != nil || width != 80 || height != 40 {
		t.Fatalf("size = %dx%d, %v", width, height, err)
	}
}

func TestAnInvalidLaterWindowEndsInputWithoutChangingSize(t *testing.T) {
	h := &host{window: charmssh.Window{Width: 80, Height: 24}}
	_, changed, err := h.resize(charmssh.Window{Width: program.MaxCells, Height: 2})
	if !errors.Is(err, ErrWindowSize) || changed {
		t.Fatalf("resize = changed %t, error %v", changed, err)
	}
	width, height, _ := h.Size()
	if width != 80 || height != 24 {
		t.Fatalf("invalid update changed size to %dx%d", width, height)
	}
}

func TestEventSourceDecodesBytesThenReportsItsResult(t *testing.T) {
	reader := make(chanReader)
	windows := make(chan charmssh.Window)
	source := newEventSource(t.Context().Done(), reader, windows,
		func(window charmssh.Window) (input.Resize, bool, error) {
			return input.Resize{Width: window.Width, Height: window.Height}, true, nil
		}, nil)
	t.Cleanup(source.Close)

	reader <- readResult{data: []byte("a")}
	event := receiveEvent(t, source.Events())
	key, ok := event.(input.Key)
	if !ok || key.Rune != 'a' || key.At.IsZero() {
		t.Fatalf("event = %#v", event)
	}

	windows <- charmssh.Window{Width: 100, Height: 30}
	if got := receiveEvent(t, source.Events()); got != (input.Resize{Width: 100, Height: 30}) {
		t.Fatalf("event = %#v", got)
	}

	want := errors.New("connection lost")
	reader <- readResult{err: want}
	if _, ok := <-source.Events(); ok {
		t.Fatal("events remained open after the read failed")
	}
	if !errors.Is(source.Err(), want) {
		t.Fatalf("error = %v, want %v", source.Err(), want)
	}
}

func TestResizeIntakeKeepsTheLatestWindow(t *testing.T) {
	reader := make(chanReader)
	windows := make(chan charmssh.Window, 32)
	observed := make(chan struct{}, 32)
	source := newEventSource(t.Context().Done(), reader, windows,
		func(window charmssh.Window) (input.Resize, bool, error) {
			observed <- struct{}{}
			return input.Resize{Width: window.Width, Height: window.Height}, true, nil
		}, nil)
	t.Cleanup(source.Close)

	for width := 81; width <= 100; width++ {
		windows <- charmssh.Window{Width: width, Height: 24}
	}
	for range 20 {
		select {
		case <-observed:
		case <-time.After(time.Second):
			t.Fatal("window intake stopped behind the event consumer")
		}
	}
	want := input.Resize{Width: 100, Height: 24}
	var last input.Event
	for last != want {
		last = receiveEvent(t, source.Events())
	}
	reader <- readResult{err: io.EOF}
	for event := range source.Events() {
		t.Errorf("unexpected event after EOF: %#v", event)
	}
}

func receiveEvent(t *testing.T, events <-chan input.Event) input.Event {
	t.Helper()
	select {
	case event, ok := <-events:
		if !ok {
			t.Fatal("event stream closed while waiting for an event")
		}
		return event
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event")
		return nil
	}
}

type readResult struct {
	data []byte
	err  error
}

type chanReader chan readResult

func (r chanReader) Read(p []byte) (int, error) {
	result := <-r
	return copy(p, result.data), result.err
}

type quittingComponent struct{ runtime *program.Runtime }

func (c quittingComponent) Draw(grid.View)        { c.runtime.Quit() }
func (quittingComponent) Handle(input.Event) bool { return false }

type fakeSession struct {
	charmssh.Session
	ctx       charmssh.Context
	window    charmssh.Window
	windows   <-chan charmssh.Window
	ptyOK     bool
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

var _ io.Reader = chanReader(nil)
