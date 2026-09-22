package ptytest

import (
	"context"
	"errors"
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
