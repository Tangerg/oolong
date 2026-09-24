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
	// cannot carry. See [Run].
	ErrLineFeed = errors.New("ssh: the session cannot carry a line feed")
)

// Run runs cfg on session until the program stops, the client disconnects or the
// transport fails.
//
// session must already have an accepted PTY, and the server must be handling it the
// default way — see [charm.land/ssh.Server.PtyHandler]. A server configured with
// [charm.land/ssh.AllocatePty] gives the session a terminal of its own and, before
// the handler runs, starts copying the channel into it and draining its window
// changes. Both of those are Run's: one channel with two readers gives each keystroke
// to whichever got there first, and a window change taken by the other consumer never
// reaches the interface at all. That is [ErrAllocatedPTY], and it is refused rather
// than raced.
//
// The default handling emulates the terminal, which is only a writer: it turns a line
// feed into a carriage return and a line feed, and then turns a doubled carriage
// return back into one. Reads and window changes it does not touch at all.
//
// A frame is exact bytes and would not survive being rewritten. Nothing this library
// composes contains a line feed that is not already the second half of one — the
// alternate screen addresses every row, and an inline block writes only pairs — so
// there is nothing in a frame for the emulation to find. A [grid.Painter] is the one
// thing that writes bytes this library did not compose, and one is free to write a
// line feed of its own: a carriage return would be added to it, and what it drew next
// would move to the first column.
//
// So it is refused rather than rewritten, with [ErrLineFeed]. The session cannot
// carry that byte, and a transport that cannot carry something has to say so — the
// alternative is a frame that arrives changed with the terminal to blame for it.
//
// Run owns Oolong's input decoder, frame writer and terminal modes for the duration of
// the call, but it does not own the SSH channel itself and does not choose an exit
// status. The surrounding SSH handler retains those decisions and can report a non-nil
// result before returning.
//
// The session's input, however, is Run's for the session's lifetime and not only for
// the call. An SSH channel cannot be read with a deadline, so a read already in
// flight when the program stops ends when the channel does. A handler that reads the
// session itself after Run returns would be taking bytes from that read; ending the
// session, by returning or by [charm.land/ssh.Session.Exit], is what it is for.
//
// The zero cfg.Color is resolved from the client's PTY environment
// rather than the server process environment. Terminal modes, character locale,
// wheel scaling and clipboard transport follow that same client-owned environment.
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
		// Reported as bytes written, because nothing was: a frame is applied whole
		// and a partial one is not a smaller frame.
		return 0, fmt.Errorf("%w: byte %d", ErrLineFeed, at)
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
