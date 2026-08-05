// Package ptytest drives a terminal program on a real pty and says what reached
// the terminal.
//
// It is the harness for the tests a [program.Host] cannot write. A host proves
// that an interface draws the frame it meant to; only a pty proves that the bytes
// of that frame do to a terminal what they were supposed to — that the block
// shrank without leaving debris, that the caret is on the row the cursor was
// placed on, that every mode the session turned on was turned off again.
//
// # What it is not
//
// A terminal emulator. It captures a byte stream and gives you assertions over
// it, which is enough for the questions worth asking about a renderer and far
// short of what it would take to answer "what does the screen look like". A test
// that needs that should say what sequence it expects instead, which is a better
// test anyway: it fails naming the thing that changed.
//
// # Where it lives
//
// Beside the library rather than inside it. It is the only thing here allowed to
// spawn a process, and nothing in the library imports it — a harness that the
// thing it tests depends on is a harness that cannot be changed.
package ptytest

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
)

// ErrUnsupported is reported by [Start] on a platform with no pty here.
var ErrUnsupported = errors.New("ptytest: no pty on this platform")

// Size is a terminal size in cells.
type Size struct{ Cols, Rows int }

// check rejects a size the winsize ioctl cannot carry.
//
// Zero is refused at the door rather than passed on: a terminal collapsed to no
// size is a state a test almost never means to ask for and always struggles to
// diagnose, because everything downstream simply draws nothing.
func (s Size) check() error {
	if s.Cols <= 0 || s.Rows <= 0 {
		return fmt.Errorf("ptytest: size %dx%d: both must be positive", s.Cols, s.Rows)
	}
	if s.Cols > 0xffff || s.Rows > 0xffff {
		return fmt.Errorf("ptytest: size %dx%d: larger than a terminal can report", s.Cols, s.Rows)
	}
	return nil
}

// Options configures a session.
type Options struct {
	// Size is the terminal's opening size. The zero value is 80 by 24.
	Size Size
	// Dir is the child's working directory, and Env its environment. Both follow
	// os/exec's rules: empty means inherit.
	Dir string
	Env []string
}

// Session is a command running on a pty, with everything it has written captured
// as it arrives.
//
// Writing and resizing are safe while it runs, which is the point: a test types,
// waits for what it typed to come back, and types again.
type Session struct {
	primary    *os.File
	cmd        *exec.Cmd
	transcript *Transcript

	readDone chan struct{}
	waitDone chan struct{}

	waitOnce sync.Once
	waitErr  error

	closeOnce sync.Once
}

// Start runs a command on a new pty of the default size.
func Start(name string, args ...string) (*Session, error) {
	return StartWith(Options{}, name, args...)
}

// StartWith runs a command on a new pty.
func StartWith(opts Options, name string, args ...string) (*Session, error) {
	size := opts.Size
	if size == (Size{}) {
		size = Size{Cols: 80, Rows: 24}
	}
	if err := size.check(); err != nil {
		return nil, err
	}

	primary, replica, err := openPTY()
	if err != nil {
		return nil, err
	}
	// The replica belongs to the child once it is running; this side of it is
	// closed either way, and closing it on the failure path is what stops a failed
	// start from leaking a descriptor per attempt.
	defer replica.Close()

	if err := setSize(primary, size); err != nil {
		primary.Close()
		return nil, err
	}

	cmd := exec.Command(name, args...)
	cmd.Dir = opts.Dir
	cmd.Env = opts.Env
	attach(cmd, replica)

	if err := cmd.Start(); err != nil {
		primary.Close()
		return nil, fmt.Errorf("ptytest: start %s: %w", name, err)
	}

	s := &Session{
		primary:    primary,
		cmd:        cmd,
		transcript: newTranscript(),
		readDone:   make(chan struct{}),
		waitDone:   make(chan struct{}),
	}
	go s.read()
	go s.wait()
	return s, nil
}

// read drains the pty into the transcript until the child is gone.
func (s *Session) read() {
	defer close(s.readDone)
	buf := make([]byte, 4096)
	for {
		n, err := s.primary.Read(buf)
		if n > 0 {
			s.transcript.append(buf[:n])
		}
		if err != nil {
			return
		}
	}
}

// wait reaps the child once, so Wait and Close can both ask.
func (s *Session) wait() {
	defer close(s.waitDone)
	err := s.cmd.Wait()
	if err != nil && !readClosed(err) {
		s.waitErr = err
	}
}

// Transcript is everything the program has written, as it arrives.
func (s *Session) Transcript() *Transcript { return s.transcript }

// Write sends bytes to the program as though they were typed.
func (s *Session) Write(p []byte) (int, error) { return s.primary.Write(p) }

// Type sends text to the program as though it were typed.
func (s *Session) Type(text string) error {
	_, err := io.WriteString(s.primary, text)
	return err
}

// Resize changes the terminal's size and tells the program, the way a terminal
// emulator would: the winsize first, then the signal.
func (s *Session) Resize(size Size) error {
	if err := size.check(); err != nil {
		return err
	}
	if err := setSize(s.primary, size); err != nil {
		return err
	}
	return signalResize(s.cmd.Process)
}

// Wait blocks until the program exits, or until ctx is done.
func (s *Session) Wait(ctx context.Context) error {
	select {
	case <-s.waitDone:
		return s.waitErr
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Close ends the session: the program is killed if it is still running, the pty
// is closed, and the goroutines reading it have finished by the time this
// returns.
//
// It is idempotent, so a test can defer it and still close explicitly.
func (s *Session) Close() error {
	s.closeOnce.Do(func() {
		select {
		case <-s.waitDone:
		default:
			if s.cmd.Process != nil {
				_ = s.cmd.Process.Kill()
			}
		}
		// Closing the primary is what ends the read: a blocking read on a pty
		// cannot be cancelled, and the child is already gone or going.
		_ = s.primary.Close()
		<-s.readDone
		<-s.waitDone
	})
	return s.waitErr
}
