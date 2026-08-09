package ssh

import (
	"errors"
	"fmt"
	"strings"

	charmssh "charm.land/ssh"

	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/program"
	"github.com/Tangerg/oolong/core/term"
)

var (
	// ErrNoPTY means the accepted SSH session did not request a terminal. Oolong
	// draws cells and therefore cannot run as a plain exec byte stream.
	ErrNoPTY = errors.New("ssh: session has no PTY")
	// ErrHostSet means the program configuration already names another transport.
	// Run never silently replaces one host with another.
	ErrHostSet = errors.New("ssh: program host is already set")
	// ErrWindowSize means an SSH window cannot safely describe a cell surface.
	ErrWindowSize = errors.New("ssh: invalid PTY window size")
)

// Run runs cfg on session until the program stops, the client disconnects or the
// transport fails.
//
// session must already have an accepted PTY. Run owns Oolong's input decoder, frame
// writer and terminal modes for the duration of the call, but it does not own the
// SSH channel itself and does not choose an exit status. The surrounding SSH
// handler retains those decisions and can report a non-nil result before returning.
//
// The zero cfg.Color, [grid.Auto], is resolved from the client's PTY environment
// rather than the server process environment.
func Run(session charmssh.Session, cfg program.Config) (err error) {
	if cfg.Host != nil {
		return ErrHostSet
	}
	if validationErr := cfg.Validate(); validationErr != nil {
		return validationErr
	}
	pty, windows, ok := session.Pty()
	if !ok {
		return ErrNoPTY
	}
	if sizeErr := validateInitialWindow(pty.Window); sizeErr != nil {
		return sizeErr
	}

	env := newEnvironment(session.Environ())
	env.set("TERM", pty.Term)
	if cfg.Color == grid.Auto {
		cfg.Color = term.DetectDepthIn(env.lookup)
	}

	options := cfg.Terminal
	// A root program owns an alternate screen on every transport. program.Run makes
	// the same decision for a local terminal; a supplied Host deliberately leaves
	// transport setup to its adapter.
	options.AltScreen = cfg.Root != nil
	ctx := session.Context()
	host := newHost(ctx.Done(), session, pty.Window, windows, options.Modes(env.lookup))
	defer func() { err = errors.Join(err, host.Close()) }()
	cfg.Host = host
	return program.Run(ctx, cfg)
}

func validateInitialWindow(window charmssh.Window) error {
	if err := program.ValidateSize(window.Width, window.Height); err != nil {
		return fmt.Errorf("%w: %w", ErrWindowSize, err)
	}
	return nil
}

// environment owns a session's last value for each variable. SSH environment
// requests are ordered, and the last request has the same meaning as the last
// assignment in a process environment.
type environment map[string]string

func newEnvironment(entries []string) environment {
	e := make(environment, len(entries)+1)
	for _, entry := range entries {
		name, value, ok := strings.Cut(entry, "=")
		if ok && name != "" {
			e[name] = value
		}
	}
	return e
}

func (e environment) set(name, value string) {
	if name != "" {
		e[name] = value
	}
}

func (e environment) lookup(name string) (string, bool) {
	value, ok := e[name]
	return value, ok
}
