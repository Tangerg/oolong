// Package ptytest drives a terminal program on a real pty and says what reached
// the terminal.
//
// It is the harness for tests an in-process fake cannot write. A fake proves that
// an interface draws the frame it meant to; only a pty proves that the bytes
// of that frame do to a terminal what they were supposed to — that the block
// shrank without leaving debris, that the caret is on the row the cursor was
// placed on, that every mode the session turned on was turned off again.
//
// # What it is not
//
// A terminal emulator. It captures a byte stream and gives you assertions over it.
// [Screen] can additionally answer which text an Oolong renderer left in each cell,
// but deliberately does not model terminal queries, input, alternate-buffer ownership,
// arbitrary painters, or the rest of a terminal's state. Tests of a particular
// protocol sequence should still assert that sequence directly: it fails naming the
// thing that changed.
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
	"math"
	"os"
	"os/exec"
	"sync"
)

// ErrUnsupported is reported by [Start] on a platform with no pty here.
var ErrUnsupported = errors.New("ptytest: no pty on this platform")

// Supported reports whether this platform has a pty here.
//
// It is worth asking before doing the work a pty test needs — building the
// binary under test, usually — rather than after: a skip that happens at the end
// of the expensive part is a skip that still costs what it skipped.
func Supported() bool { return supported }

// Size is a terminal size in cells.
type Size struct{ Cols, Rows int }

// check rejects a size the winsize ioctl cannot carry.
//
// Zero is refused at the door rather than passed on: a terminal collapsed to no
// size is a state a test almost never means to ask for and always struggles to
// diagnose, because everything downstream simply draws nothing.
func (s Size) check() error {
	_, _, err := s.dims()
	return err
}

// dims is the size in the type the winsize ioctl takes, or the reason it is not a
// size at all.
//
// The bound and the narrowing are in one function on purpose: a conversion whose
// safety depends on a check somewhere else is a conversion nobody can read.
func (s Size) dims() (cols, rows uint16, err error) {
	if s.Cols <= 0 || s.Rows <= 0 {
		return 0, 0, fmt.Errorf("ptytest: size %dx%d: both must be positive", s.Cols, s.Rows)
	}
	if s.Cols > math.MaxUint16 || s.Rows > math.MaxUint16 {
		return 0, 0, fmt.Errorf("ptytest: size %dx%d: larger than a terminal can report", s.Cols, s.Rows)
	}
	return uint16(s.Cols), uint16(s.Rows), nil
}

// Config describes a session before it starts.
type Config struct {
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
	process    *os.Process
	transcript *Transcript

	readDone chan struct{}
	waitDone chan struct{}

	// waitErr is written by the one goroutine that reaps the child, before
	// waitDone closes, and read only after — which is what makes it safe without
	// a lock.
	waitErr error

	closeOnce sync.Once
}

// Start runs a command on a new pty.
//
// Cancelling ctx kills the program. A test that hands it t.Context() therefore
// cannot leave one behind however it fails — which matters more here than
// anywhere else in the repository, because what would be left behind is a process
// holding a terminal.
func Start(ctx context.Context, cfg Config, name string, args ...string) (*Session, error) {
	size := cfg.Size
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
	defer func() { _ = replica.Close() }()

	if err := setSize(primary, size); err != nil {
		_ = primary.Close()
		return nil, err
	}

	// Running a command the caller named is what this package is for.
	//nolint:gosec // G204: the command is the harness's argument, by design.
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = cfg.Dir
	cmd.Env = cfg.Env
	attach(cmd, replica)

	if err := cmd.Start(); err != nil {
		_ = primary.Close()
		return nil, fmt.Errorf("ptytest: start %s: %w", name, err)
	}

	s := &Session{
		primary:    primary,
		process:    cmd.Process,
		transcript: newTranscript(),
		readDone:   make(chan struct{}),
		waitDone:   make(chan struct{}),
	}
	go s.read()
	go s.wait(cmd)
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
func (s *Session) wait(cmd *exec.Cmd) {
	defer close(s.waitDone)
	// The waiting goroutine is the only owner of exec.Cmd after Start returns.
	// Once Wait finishes, its arguments, environment and launch bookkeeping can be
	// collected even while the Session remains available for transcript assertions.
	err := cmd.Wait()
	if err != nil && !readClosed(err) {
		s.waitErr = err
	}
}

// Transcript is everything the program has written, as it arrives.
func (s *Session) Transcript() *Transcript { return s.transcript }

// Write sends bytes to the program as though they were typed.
func (s *Session) Write(p []byte) (int, error) { return s.primary.Write(p) }

// Resize changes the terminal's size and tells the program, the way a terminal
// emulator would: the winsize first, then the signal.
func (s *Session) Resize(size Size) error {
	if err := size.check(); err != nil {
		return err
	}
	if err := setSize(s.primary, size); err != nil {
		return err
	}
	return signalResize(s.process)
}

// Wait blocks until the program exits. If ctx ends first, Wait returns
// [context.Cause] of ctx.
func (s *Session) Wait(ctx context.Context) error {
	select {
	case <-s.waitDone:
		return s.waitErr
	case <-ctx.Done():
		return context.Cause(ctx)
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
			if s.process != nil {
				_ = s.process.Kill()
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
