// Package ptytest verifies terminal bytes through a real PTY. [Screen] models Oolong
// cell output, excluding terminal queries, input, alternate-buffer lifetime and
// opaque painters; assert protocol sequences directly for those contracts. The
// harness stays outside runtime dependencies so it can vary independently.
package ptytest

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"sync"
	"syscall"

	"github.com/Tangerg/oolong/ui/core/process"
)

// ErrUnsupported is reported by [Start] on a platform with no pty here.
var ErrUnsupported = errors.New("ptytest: no pty on this platform")

// Supported lets a test skip before paying to build a child it cannot launch.
func Supported() bool { return supported }

// Size is a terminal size in cells.
type Size struct{ Cols, Rows int }

const (
	defaultColumns = 80
	defaultRows    = 24
)

// A zero-sized terminal silently draws nothing, hiding the invalid test setup.
func (s Size) check() error {
	_, _, err := s.dims()
	return err
}

// The bound and narrowing stay together so no caller can bypass the ioctl's limit.
func (s Size) dims() (cols, rows uint16, err error) {
	if s.Cols <= 0 || s.Rows <= 0 {
		return 0, 0, fmt.Errorf("ptytest: size %dx%d: both must be positive", s.Cols, s.Rows)
	}
	if s.Cols > math.MaxUint16 || s.Rows > math.MaxUint16 {
		return 0, 0, fmt.Errorf("ptytest: size %dx%d: larger than a terminal can report", s.Cols, s.Rows)
	}
	return uint16(s.Cols), uint16(s.Rows), nil
}

type Config struct {
	// Size is the terminal's opening size. The zero value is 80 by 24.
	Size Size
	// Dir is the child's working directory; empty means this process's.
	Dir string
	// Env is the child's whole environment. Nil inherits this process's, and any
	// other value replaces it — including an empty non-nil slice, which is how a
	// test asks for a child with no environment at all. The distinction is
	// os/exec's and is kept rather than smoothed over: a harness that turned an
	// empty slice back into inheritance would silently answer a hermetic test with
	// the developer's shell.
	Env []string
}

// Session is a command running on a pty, with everything it has written captured
// as it arrives.
//
// Writing and resizing are safe while it runs, which is the point: a test types,
// waits for what it typed to come back, and types again. A Session must not be copied
// after construction; its child, pty and reaping goroutines are one lifetime.
type Session struct {
	primary    *os.File
	process    *os.Process
	transcript *Transcript

	waitDone chan struct{}

	// The observer publishes exit before waitDone; reaping is held until release
	// and published before reapDone. Readers synchronize through those channels.
	waitErr  error
	release  chan struct{}
	reapDone chan struct{}
	reapErr  error

	stopCancel func() bool
	closeOnce  sync.Once
	closeErr   error
}

// Start runs a command on a new pty.
//
// Cancelling ctx closes the session and kills its process group, even if
// descendants retain the terminal after the direct child exits. Call Close to
// collect teardown errors or to end the session before ctx.
func Start(ctx context.Context, cfg Config, name string, args ...string) (*Session, error) {
	size := cfg.Size
	if size == (Size{}) {
		size = Size{Cols: defaultColumns, Rows: defaultRows}
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

	//nolint:gosec // G204: the command is the harness's argument, by design.
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = cfg.Dir
	cmd.Env = cfg.Env
	attach(cmd, replica)
	// CommandContext checks cancellation during Start. The session owns all later
	// cancellation: an exec.Cmd watcher stops as soon as the direct child exits.
	cmd.Cancel = nil

	if err := cmd.Start(); err != nil {
		_ = primary.Close()
		return nil, fmt.Errorf("ptytest: start %s: %w", name, err)
	}

	s := &Session{
		primary:    primary,
		process:    cmd.Process,
		transcript: newTranscript(),
		waitDone:   make(chan struct{}),
		release:    make(chan struct{}),
		reapDone:   make(chan struct{}),
	}
	go s.read()
	go s.wait(cmd)
	s.stopCancel = context.AfterFunc(ctx, s.close)
	return s, nil
}

func (s *Session) read() {
	buf := make([]byte, 4096)
	for {
		n, err := s.primary.Read(buf)
		if n > 0 {
			s.transcript.append(buf[:n])
		}
		if err != nil {
			// Close unblocks this read by closing under it, which is an ending and
			// not a failure — the same as the far end going away.
			if errors.Is(err, io.EOF) || errors.Is(err, os.ErrClosed) || readClosed(err) {
				err = nil
			}
			s.transcript.finish(err)
			return
		}
	}
}

func (s *Session) wait(cmd *exec.Cmd) {
	// Keeping the exited leader waitable reserves the group identity until Close
	// has signaled every descendant. Wait must never release that reservation.
	s.waitErr = process.WaitExit(cmd.Process)
	close(s.waitDone)
	<-s.release
	if _, exited := errors.AsType[*process.ExitError](s.waitErr); s.waitErr != nil && !exited {
		s.reapErr = errors.Join(s.waitErr, cmd.Process.Release())
		_ = cmd.Wait()
		close(s.reapDone)
		return
	}
	s.reapErr = cmd.Wait()
	close(s.reapDone)
}

func (s *Session) endProcessGroup() error {
	select {
	case <-s.waitDone:
		if _, exited := errors.AsType[*process.ExitError](s.waitErr); s.waitErr != nil && !exited {
			return s.waitErr
		}
	default:
	}
	if s.process == nil {
		return nil
	}
	if err := process.SignalGroup(s.process, syscall.SIGKILL); err != nil {
		return fmt.Errorf("ptytest: terminate process group: %w", err)
	}
	return nil
}

func (s *Session) Transcript() *Transcript { return s.transcript }

func (s *Session) Write(p []byte) (int, error) { return s.primary.Write(p) }

// Resize changes the PTY geometry. The kernel notifies the current foreground
// process group when the size changes, even after the original child exits.
func (s *Session) Resize(size Size) error {
	return setSize(s.primary, size)
}

// Wait observes exit without reaping: Close must still release the child identity.
// A failed child returns *process.ExitError. If ctx ends first, its cause is returned.
func (s *Session) Wait(ctx context.Context) error {
	select {
	case <-s.waitDone:
		return s.waitErr
	case <-ctx.Done():
		return context.Cause(ctx)
	}
}

// Drain waits until the PTY reader has collected all output and reached EOF.
// Call it after Wait and before final transcript assertions. The context must
// bound the wait: a descendant can retain the terminal after the direct child exits.
func (s *Session) Drain(ctx context.Context) error {
	select {
	case <-s.transcript.done:
		return s.transcript.err
	case <-ctx.Done():
		return context.Cause(ctx)
	}
}

// Close kills the process group, closes the PTY and joins its readers. Group cleanup
// prevents descendants from retaining the terminal. Calls are idempotent and preserve
// process, reader and teardown errors. Failed group termination closes the PTY and
// returns the failure without waiting indefinitely for an unstopped process.
func (s *Session) Close() error {
	if s.stopCancel != nil {
		s.stopCancel()
	}
	s.close()
	return s.closeErr
}

func (s *Session) close() {
	s.closeOnce.Do(func() {
		// Whether the program is still running says nothing about the session it
		// started. A child that exited having left something of its own behind left
		// it in the same group and holding the same terminal, and waiting for the
		// leader is not waiting for that.
		endErr := s.endProcessGroup()
		close(s.release)
		// Closing the primary is what ends the read. It ends it even when something
		// outside this session still holds the replica: the descriptor is in the
		// runtime's poller, so the pending read is unblocked rather than waited for.
		closeErr := s.primary.Close()
		<-s.transcript.done
		var waitErr error
		if endErr == nil {
			<-s.reapDone
			waitErr = s.reapErr
		} else {
			select {
			case <-s.reapDone:
				waitErr = s.reapErr
			default:
			}
		}
		s.closeErr = errors.Join(endErr, closeErr, s.transcript.err, waitErr)
	})
}
