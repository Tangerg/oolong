package ptytest

import (
	"bytes"
	"context"
	"errors"
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
	mu  sync.Mutex
	buf bytes.Buffer
	// size is the terminal's current geometry and from is where the output laid out
	// for it begins: a resize starts a new geometry, and what was written before it
	// was laid out for another size.
	size Size
	from int
	grew chan struct{}
	done chan struct{}
	err  error // Written by the PTY reader before done closes.
}

func newTranscript(size Size) *Transcript {
	return &Transcript{size: size, grew: make(chan struct{}), done: make(chan struct{})}
}

// resized starts the output for a new terminal geometry.
func (t *Transcript) resized(size Size) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.size, t.from = size, t.buf.Len()
}

// current is one consistent snapshot of the geometry, its output and the wake-up
// for the next byte.
func (t *Transcript) current() (Size, []byte, <-chan struct{}) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.size, bytes.Clone(t.buf.Bytes()[t.from:]), t.grew
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

// Screen interprets the output written since the latest [Session.Resize] as
// cell-renderer output at the terminal's current size. An interface repaints for a
// new size, and the output before it was laid out for another one, so it is not part
// of the screen.
//
// It takes one byte snapshot before decoding, so output arriving concurrently belongs
// wholly to this snapshot or the next one. See [Screen] for the intentionally narrow
// boundary of the interpretation.
func (t *Transcript) Screen() (*Screen, error) {
	size, written, _ := t.current()
	return decodeScreen(size, written)
}

// WaitForScreen waits until the [Transcript.Screen] text, its rows joined by newlines,
// shows every token, and returns that screen. A frame still arriving may not decode
// yet, so a decoding failure is waited past rather than returned. A missing token
// reports what the screen last showed together with the reader failure, [io.EOF]
// when output ended cleanly, or [context.Cause] when the caller stopped waiting.
func (t *Transcript) WaitForScreen(ctx context.Context, tokens ...string) (*Screen, error) {
	var cause error
	for {
		size, written, grew := t.current()
		screen, decodeErr := decodeScreen(size, written)
		var shown string
		if decodeErr == nil {
			shown = strings.Join(screen.Rows(), "\n")
			if containsAll(shown, tokens) {
				return screen, nil
			}
		}
		if cause != nil {
			return nil, &missing{tokens: absent(shown, tokens), seen: "the screen showed", text: shown, err: errors.Join(cause, decodeErr)}
		}
		select {
		case <-grew:
		case <-t.done:
			cause = t.ended()
		case <-ctx.Done():
			cause = context.Cause(ctx)
		}
	}
}

func decodeScreen(size Size, written []byte) (*Screen, error) {
	screen, err := NewScreen(size)
	if err != nil {
		return nil, err
	}
	if err := screen.Apply(written); err != nil {
		return nil, err
	}
	if err := screen.Flush(); err != nil {
		return nil, err
	}
	return screen, nil
}

// ended is why output stopped: the reader's failure, or EOF for a clean end.
func (t *Transcript) ended() error {
	if t.err != nil {
		return t.err
	}
	return io.EOF
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
			return &missing{tokens: absent(text, tokens), seen: "what arrived was", text: text, err: cause}
		}
		select {
		case <-grew:
		case <-t.done:
			cause = t.ended()
		case <-ctx.Done():
			cause = context.Cause(ctx)
		}
	}
}

// missing names the tokens that never appeared, why waiting stopped, and the text
// they were looked for in; seen says whether that text is raw output or a screen.
type missing struct {
	tokens []string
	seen   string
	text   string
	err    error
}

func (m *missing) Error() string {
	return "ptytest: never saw " + strings.Join(quoteAll(m.tokens), ", ") +
		": " + m.err.Error() + "\n" + m.seen + ":\n" + strconv.Quote(m.text)
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
