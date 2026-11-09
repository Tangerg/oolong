package ssh

import (
	"errors"
	"reflect"
	"sync"

	charmssh "charm.land/ssh"

	"github.com/Tangerg/oolong/ui/core/clipboard"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/term"
)

// The PTY slot belongs to the server, so competing installers cannot coordinate
// with a lock on either adapter. Registration ends before the server starts.
var installMu sync.Mutex

// Adapter binds the PTY request and the application handler for each SSH channel.
// Install one adapter per server before Serve. A connection may multiplex sessions.
// An Adapter must not be copied after installation.
type Adapter struct {
	mu       sync.Mutex
	sessions map[charmssh.Session]*terminalBinding
}

// Install claims the vacant PTY handler for emulation. PTY admission belongs to
// server.PtyCallback, which runs before the SSH request loop commits a channel to a PTY.
func (a *Adapter) Install(server *charmssh.Server) error {
	if server == nil {
		return errors.New("ssh: server is required")
	}
	installMu.Lock()
	defer installMu.Unlock()
	if server.PtyHandler != nil {
		return errors.New("ssh: server PTY handler already installed")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.sessions != nil {
		return errors.New("ssh: adapter already installed")
	}
	if err := charmssh.EmulatePty()(server); err != nil {
		return err
	}
	emulate := server.PtyHandler
	a.sessions = make(map[charmssh.Session]*terminalBinding)
	server.PtyHandler = func(ctx charmssh.Context, session charmssh.Session, request charmssh.Pty) (func() error, error) {
		if session == nil || !reflect.ValueOf(session).Comparable() {
			return nil, errors.New("ssh: PTY session has no comparable channel identity")
		}
		release, err := emulate(ctx, session, request)
		if err != nil {
			return nil, err
		}
		terminal := captureTerminal(session)
		a.mu.Lock()
		a.sessions[session] = terminal
		a.mu.Unlock()
		return func() error {
			terminal.release()
			a.mu.Lock()
			delete(a.sessions, session)
			a.mu.Unlock()
			return release()
		}, nil
	}
	return nil
}

// Bind takes the original session supplied by the server handler. Keep the returned
// handle before decorating that session; channel identity does not depend on wrappers
// used for authentication, logging or routing.
func (a *Adapter) Bind(session charmssh.Session) (*Terminal, error) {
	if session == nil || !reflect.ValueOf(session).Comparable() {
		return nil, errors.New("ssh: original handler session required")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.sessions == nil {
		return nil, errors.New("ssh: adapter is not installed")
	}
	terminal := a.sessions[session]
	if terminal == nil {
		return nil, ErrNoPTY
	}
	if terminal.err != nil {
		return nil, terminal.err
	}
	return &Terminal{binding: terminal}, nil
}

// Terminal is the emulated PTY accepted for one channel. Its window listener
// starts during the PTY request. Terminal modes and input belong to a later Open.
type Terminal struct{ binding *terminalBinding }

type terminalBinding struct {
	session charmssh.Session
	name    string
	windows *windowFeed
	err     error

	mu          sync.Mutex
	unavailable bool
}

func captureTerminal(session charmssh.Session) *terminalBinding {
	p, windows, ok := session.Pty()
	t := &terminalBinding{session: session, name: p.Term}
	switch {
	case !ok:
		t.err = ErrNoPTY
	case !session.EmulatedPty() || !p.IsZero():
		t.err = ErrAllocatedPTY
	default:
		t.windows = newWindowFeed(p.Window, windows)
	}
	return t
}

func (t *terminalBinding) release() {
	t.mu.Lock()
	t.unavailable = true
	t.mu.Unlock()
	if t.windows != nil {
		t.windows.stop()
	}
}

// Open acquires input and modes once, after the application accepted the session.
// Close the host after Run. The surrounding SSH handler owns exit status and the channel.
func (t *Terminal) Open(cfg term.Config) (*Host, error) {
	if t == nil || t.binding == nil {
		return nil, ErrNoPTY
	}
	return t.binding.open(cfg)
}

func (t *terminalBinding) open(cfg term.Config) (*Host, error) {
	if err := cfg.RestoreCursor.Shape.Validate(); err != nil {
		return nil, err
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.unavailable {
		return nil, errors.New("ssh: terminal already released or opened")
	}
	if err := t.windows.result(); err != nil {
		return nil, err
	}
	t.unavailable = true
	env := newEnvironment(t.session.Environ())
	env.set("TERM", t.name)
	return newHost(t.session.Context().Done(), exactly(t.session), t.windows,
		cfg.Modes(env.lookup), clipboard.New(cfg.Features.Clipboard),
		term.DetectLocale(env.lookup), term.DetectDepth(env.lookup),
		input.WheelFor(env.lookup, "")), nil
}
