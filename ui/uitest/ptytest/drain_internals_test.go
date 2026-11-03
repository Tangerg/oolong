package ptytest

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"testing"
)

func TestDrainObservesReaderCompletionAndCancellation(t *testing.T) {
	s := &Session{transcript: newTranscript()}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := s.Drain(ctx); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	want := errors.New("read failed")
	s.transcript.finish(want)
	if err := s.Transcript().WaitFor(t.Context(), "missing"); !errors.Is(err, want) {
		t.Fatalf("transcript wait discarded read failure: %v", err)
	}
	if err := s.Drain(t.Context()); !errors.Is(err, want) {
		t.Fatal(err)
	}
}

func TestCloseReportsGroupRefusalWithoutWaitingForTheLeader(t *testing.T) {
	if !Supported() {
		t.Skip("no PTY on this platform")
	}
	primary, err := os.CreateTemp(t.TempDir(), "unowned-session")
	if err != nil {
		t.Fatal(err)
	}
	s := &Session{
		primary: primary, process: &os.Process{},
		transcript: newTranscript(), waitDone: make(chan struct{}),
		release: make(chan struct{}), reapDone: make(chan struct{}),
	}
	s.transcript.finish(nil)
	if err := s.Close(); err == nil {
		t.Fatal("an unowned process group was reported stopped")
	}
}

func TestFinalReapPreservesCommandCancellationFailures(t *testing.T) {
	if !Supported() {
		t.Skip("no PTY on this platform")
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	cmd := exec.CommandContext(ctx, "sh", "-c", "read ending")
	cancelled := make(chan struct{})
	cmd.Cancel = func() error { close(cancelled); return syscall.EIO }
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = stdin.Close() }()
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	s := &Session{waitDone: make(chan struct{}), release: make(chan struct{}), reapDone: make(chan struct{})}
	go s.wait(cmd)
	release := sync.OnceFunc(func() { close(s.release) })
	defer func() { _ = cmd.Process.Kill(); release(); <-s.reapDone }()
	cancel()
	<-cancelled
	if _, err := io.WriteString(stdin, "done\n"); err != nil {
		t.Fatal(err)
	}
	if err := s.Wait(t.Context()); err != nil {
		t.Fatalf("observing successful child exit: %v", err)
	}
	release()
	<-s.reapDone
	if !errors.Is(s.reapErr, syscall.EIO) {
		t.Fatalf("final reap discarded command cancellation failure: %v", s.reapErr)
	}
}

func TestClosePreservesReaderAndProcessFailures(t *testing.T) {
	if !Supported() {
		t.Skip("no PTY on this platform")
	}
	primary, err := os.CreateTemp(t.TempDir(), "closed-session")
	if err != nil {
		t.Fatal(err)
	}
	readFailure := errors.New("transcript reader failed")
	waitFailure := errors.New("process wait failed")
	s := &Session{
		primary: primary, transcript: newTranscript(), waitDone: make(chan struct{}),
		release: make(chan struct{}), reapDone: make(chan struct{}), reapErr: waitFailure,
	}
	s.transcript.finish(readFailure)
	close(s.waitDone)
	close(s.reapDone)
	for range 2 {
		if err := s.Close(); !errors.Is(err, readFailure) || !errors.Is(err, waitFailure) {
			t.Fatalf("Close discarded a session failure: %v", err)
		}
	}
}
