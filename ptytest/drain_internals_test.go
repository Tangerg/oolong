package ptytest

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"syscall"
	"testing"
)

func TestDrainObservesReaderCompletionAndCancellation(t *testing.T) {
	s := &Session{readDone: make(chan struct{})}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := s.Drain(ctx); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	want := errors.New("read failed")
	s.readErr = want
	close(s.readDone)
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
		readDone: make(chan struct{}), waitDone: make(chan struct{}),
	}
	close(s.readDone)
	if err := s.Close(); err == nil {
		t.Fatal("an unowned process group was reported stopped")
	}
}

func TestProcessWaitDoesNotSuppressPTYReadErrors(t *testing.T) {
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
	s := &Session{waitDone: make(chan struct{})}
	go s.wait(cmd)
	defer func() { _ = cmd.Process.Kill(); <-s.waitDone }()
	cancel()
	<-cancelled
	if _, err := io.WriteString(stdin, "done\n"); err != nil {
		t.Fatal(err)
	}
	if err := s.Wait(t.Context()); !errors.Is(err, syscall.EIO) {
		t.Fatalf("process cancellation error was treated as PTY EOF: %v", err)
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
		primary: primary, readDone: make(chan struct{}), waitDone: make(chan struct{}),
		readErr: readFailure, waitErr: waitFailure,
	}
	close(s.readDone)
	close(s.waitDone)
	for range 2 {
		if err := s.Close(); !errors.Is(err, readFailure) || !errors.Is(err, waitFailure) {
			t.Fatalf("Close discarded a session failure: %v", err)
		}
	}
}
