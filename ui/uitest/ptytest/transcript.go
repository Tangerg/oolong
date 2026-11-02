package ptytest

import (
	"bytes"
	"context"
	"io"
	"strconv"
	"strings"
	"sync"
)

// Transcript is everything a session has written, accumulated as it arrives.
//
// It is safe to read while the program is still running, which is the whole point:
// a test types something, waits for the frame that answers it, and types again. A
// Transcript must not be copied after first use; its buffer and wake-up generation
// are one concurrent snapshot source.
type Transcript struct {
	mu   sync.Mutex
	buf  bytes.Buffer
	grew chan struct{}
	done chan struct{}
	err  error // Written by the PTY reader before done closes.
}

func newTranscript() *Transcript {
	return &Transcript{grew: make(chan struct{}), done: make(chan struct{})}
}

func (t *Transcript) finish(err error) {
	t.err = err
	close(t.done)
}

func (t *Transcript) append(p []byte) {
	t.mu.Lock()
	t.buf.Write(p)
	// Closing and replacing rather than sending: every waiter is woken, and one
	// that has not started waiting yet does not miss the wake-up, because it will
	// check the buffer before it waits again.
	grew := t.grew
	t.grew = make(chan struct{})
	t.mu.Unlock()
	close(grew)
}

// Bytes is a snapshot of everything written so far.
func (t *Transcript) Bytes() []byte {
	t.mu.Lock()
	defer t.mu.Unlock()
	return bytes.Clone(t.buf.Bytes())
}

func (t *Transcript) String() string { return string(t.Bytes()) }

// Screen interprets the current transcript as cell-renderer output at size.
//
// It takes one byte snapshot before decoding, so output arriving concurrently belongs
// wholly to this snapshot or the next one. See [Screen] for the intentionally narrow
// boundary of the interpretation.
func (t *Transcript) Screen(size Size) (*Screen, error) {
	screen, err := NewScreen(size)
	if err != nil {
		return nil, err
	}
	if err := screen.Apply(t.Bytes()); err != nil {
		return nil, err
	}
	if err := screen.Flush(); err != nil {
		return nil, err
	}
	return screen, nil
}

// WaitFor waits for every token, in any order. A missing token reports the
// reader failure, [io.EOF] when output ended cleanly, or [context.Cause] when
// the caller stopped waiting. It checks the last bytes received before reporting absence.
func (t *Transcript) WaitFor(ctx context.Context, tokens ...string) error {
	var cause error
	for {
		t.mu.Lock()
		text := t.buf.String()
		grew := t.grew
		t.mu.Unlock()

		if containsAll(text, tokens) {
			return nil
		}
		if cause != nil {
			return &missing{tokens: absent(text, tokens), transcript: text, err: cause}
		}
		select {
		case <-grew:
		case <-t.done:
			cause = t.err
			if cause == nil {
				cause = io.EOF
			}
		case <-ctx.Done():
			cause = context.Cause(ctx)
		}
	}
}

type missing struct {
	tokens     []string
	transcript string
	err        error
}

func (m *missing) Error() string {
	return "ptytest: never saw " + strings.Join(quoteAll(m.tokens), ", ") +
		": " + m.err.Error() + "\nwhat arrived was:\n" + strconv.Quote(m.transcript)
}

func (m *missing) Unwrap() error { return m.err }

func containsAll(text string, tokens []string) bool {
	for _, token := range tokens {
		if !strings.Contains(text, token) {
			return false
		}
	}
	return true
}

func absent(text string, tokens []string) []string {
	var out []string
	for _, token := range tokens {
		if !strings.Contains(text, token) {
			out = append(out, token)
		}
	}
	return out
}
