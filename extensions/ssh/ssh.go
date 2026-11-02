package ssh

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"

	charmssh "charm.land/ssh"

	"github.com/Tangerg/oolong/ui/core/term"
)

var (
	// ErrNoPTY means the accepted SSH session did not request a terminal. Oolong
	// draws cells and therefore cannot run as a plain exec byte stream.
	ErrNoPTY      = errors.New("ssh: session has no PTY")
	ErrWindowSize = errors.New("ssh: invalid PTY window size")
	// ErrAllocatedPTY means the server gave this session a terminal of its own and
	// is already reading the channel into it. A host needs the channel to itself.
	ErrAllocatedPTY = errors.New("ssh: session's PTY is allocated by the server")
	// ErrLineFeed means a frame contained a line feed of its own, which this session
	// cannot carry. The frame is refused whole and the session is left as it was, so
	// what [Host] owns is still given back on the way out. See [Terminal.Open].
	ErrLineFeed = errors.New("ssh: the session cannot carry a line feed")
)

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

// PTY emulation rewrites a bare LF, so exact frame delivery must reject it.
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
