package ssh

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"

	charmssh "charm.land/ssh"

	"github.com/Tangerg/oolong/core/clipboard"
	"github.com/Tangerg/oolong/core/input"
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
	// ErrAllocatedPTY means the server gave this session a terminal of its own and
	// is already reading the channel into it. Run needs the channel to itself.
	ErrAllocatedPTY = errors.New("ssh: session's PTY is allocated by the server")
	// ErrLineFeed means a frame contained a line feed of its own, which this session
	// cannot carry. The frame is refused whole and the session is left as it was, so
	// what [Run] owns is still given back on the way out. See [Run].
	ErrLineFeed = errors.New("ssh: the session cannot carry a line feed")
)

// Run runs cfg on session until the program stops, the client disconnects or the
// transport fails.
//
// session must already have an accepted PTY handled the default way — see
// [charm.land/ssh.Server.PtyHandler]. A server configured with
// [charm.land/ssh.AllocatePty] starts copying the channel into a terminal of its own,
// and draining its window changes, before the handler runs. Both are Run's: two
// readers give each keystroke to whichever got there first, and a window change taken
// by the other consumer never reaches the interface. That is [ErrAllocatedPTY].
//
// The default handling is only a writer, and the one thing it rewrites is a line feed
// that is not already the second half of a carriage-return pair. Nothing this library
// composes contains one, and the one thing that can is a [grid.Painter] writing bytes
// of its own. A frame is exact bytes, so that frame is refused with [ErrLineFeed]
// rather than carried changed with the terminal to blame for it.
//
// Run owns Oolong's input decoder, frame writer and terminal modes for the duration of
// the call. It does not own the SSH channel and does not choose an exit status, which
// the surrounding handler keeps.
//
// The session's input, however, is Run's for the session's lifetime and not only for
// the call: an SSH channel cannot be read with a deadline, so a read in flight when
// the program stops ends when the channel does. A handler that reads the session
// after Run returns takes bytes from that read.
//
// The zero cfg.Color, terminal modes, character locale, wheel scaling and clipboard
// transport all resolve from the client's PTY environment rather than the server
// process environment.
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
	// A terminal the server allocated is one the library is already reading into, and
	// draining window changes for, from goroutines started before this handler ran.
	// Neither can be taken back, and sharing either of them is not something a
	// contract can paper over: the keystroke that went to the other reader is gone.
	if !session.EmulatedPty() {
		return ErrAllocatedPTY
	}
	if sizeErr := validateInitialWindow(pty.Window); sizeErr != nil {
		return sizeErr
	}

	env := newEnvironment(session.Environ())
	env.set("TERM", pty.Term)
	terminalConfig := cfg.TerminalConfig()
	// A root program owns an alternate screen on every transport. program.Run makes
	// the same decision for a local terminal; a supplied Host deliberately leaves
	// transport setup to its adapter.
	ctx := session.Context()
	host := newHost(
		ctx.Done(), exactly(session), pty.Window, windows,
		terminalConfig.Modes(env.lookup), clipboard.New(env.lookup),
		term.DetectLocale(env.lookup),
		term.DetectDepth(env.lookup),
		input.WheelFor(env.lookup, ""),
	)
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

// exactly is the session as a frame writer: its own reader, and a writer that
// refuses the one byte the session would change on its way out.
func exactly(session charmssh.Session) io.ReadWriter {
	return exactChannel{Reader: session, to: session}
}

type exactChannel struct {
	io.Reader
	to io.Writer
}

func (c exactChannel) Write(p []byte) (int, error) {
	if at := bareLineFeed(p); at >= 0 {
		// Zero bytes written, because nothing was: a frame is applied whole and a
		// partial one is not a smaller frame. The channel is untouched and still
		// carries everything else, which is the difference [term.ErrFrameRefused]
		// exists to say — a session that refused a frame has not gone away, and the
		// terminal it was given still has to be handed back.
		return 0, fmt.Errorf("%w: byte %d: %w", ErrLineFeed, at, term.ErrFrameRefused)
	}
	return c.to.Write(p)
}

// bareLineFeed is where a line feed stands on its own rather than after a carriage
// return, or -1 for nowhere. It is the emulation's own rule, read from this side.
func bareLineFeed(p []byte) int {
	for at := 0; ; {
		found := bytes.IndexByte(p[at:], '\n')
		if found < 0 {
			return -1
		}
		at += found
		if at == 0 || p[at-1] != '\r' {
			return at
		}
		at++
	}
}
