package ptytest

import (
	"bytes"
	"context"
	"strings"
	"sync"
	"time"
)

// Transcript is everything a session has written, accumulated as it arrives.
//
// It is safe to read while the program is still running, which is the whole point:
// a test types something, waits for the frame that answers it, and types again.
type Transcript struct {
	mu   sync.Mutex
	buf  bytes.Buffer
	grew chan struct{}
}

func newTranscript() *Transcript {
	return &Transcript{grew: make(chan struct{})}
}

// append adds what was just read and wakes anything waiting.
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

// String is the snapshot as a string, byte for byte.
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

// WaitFor blocks until every token has appeared, or until ctx is done.
//
// Every token, not the last one: a test waiting for a frame usually has more than
// one thing to say about it, and waiting for them one at a time would pass on a
// transcript where they arrived in the wrong order.
func (t *Transcript) WaitFor(ctx context.Context, tokens ...string) error {
	for {
		t.mu.Lock()
		text := t.buf.String()
		grew := t.grew
		t.mu.Unlock()

		if containsAll(text, tokens) {
			return nil
		}
		select {
		case <-grew:
		case <-ctx.Done():
			return &missing{tokens: absent(text, tokens), transcript: text, err: ctx.Err()}
		}
	}
}

// WaitWithin is [Transcript.WaitFor] with a deadline instead of a context, which
// is what a test usually has.
func (t *Transcript) WaitWithin(d time.Duration, tokens ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), d)
	defer cancel()
	return t.WaitFor(ctx, tokens...)
}

// missing reports what never turned up, with the transcript that did.
type missing struct {
	tokens     []string
	transcript string
	err        error
}

func (m *missing) Error() string {
	return "ptytest: never saw " + strings.Join(quoteAll(m.tokens), ", ") +
		"\nwhat arrived was:\n" + m.transcript
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
