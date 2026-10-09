package programtest

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/term"
)

type heldOutput struct {
	entered chan struct{}
	release chan struct{}
}

func (w heldOutput) Write(data []byte) (int, error) {
	close(w.entered)
	<-w.release
	return len(data), nil
}

func TestConcurrentCloseWaitsForTheSameWriterSettlement(t *testing.T) {
	output := heldOutput{entered: make(chan struct{}), release: make(chan struct{})}
	release := sync.OnceFunc(func() { close(output.release) })
	host := &Host{input: newEventSource(), writer: term.NewWriter(output)}
	defer func() { _ = host.Close() }()
	defer release()
	host.writer.Queue([]byte("frame"))
	<-output.entered
	first := make(chan error, 1)
	go func() { first <- host.Close() }()
	<-host.input.ended
	second := make(chan error, 1)
	started := make(chan struct{})
	go func() {
		close(started)
		second <- host.Close()
	}()
	<-started
	returned := false
	select {
	case err := <-second:
		returned = true
		t.Errorf("concurrent Close returned before the writer settled: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	release()
	if err := <-first; err != nil {
		t.Fatal(err)
	}
	if !returned {
		if err := <-second; err != nil {
			t.Fatal(err)
		}
	}
}

type failedOutput struct{ err error }

func (w failedOutput) Write([]byte) (int, error) { return 0, w.err }

func TestClosePreservesTheWriterFailureOnEveryCall(t *testing.T) {
	want := errors.New("destination failed")
	host := &Host{input: newEventSource(), writer: term.NewWriter(failedOutput{err: want})}
	host.writer.Queue([]byte("frame"))
	for range 2 {
		if err := host.Close(); !errors.Is(err, want) {
			t.Errorf("Close = %v, want %v", err, want)
		}
	}
}

func TestRefusedResizeCannotChangeHostSize(t *testing.T) {
	host := New(t, Config{Width: 8, Height: 3})
	host.input.close()
	if host.Resize(20, 10) {
		t.Fatal("closed host accepted a resize")
	}
	width, height, err := host.Size()
	if err != nil || width != 8 || height != 3 {
		t.Fatalf("refused resize changed size: %d, %d, %v", width, height, err)
	}
}

func TestSentResizeUsesTheHostSizeOwner(t *testing.T) {
	host := New(t, Config{Width: 8, Height: 3})
	resized := input.Resize{Width: 20, Height: 10}
	if !host.Send(resized) {
		t.Fatal("host refused valid resize input")
	}
	if got := <-host.Events(); got != resized {
		t.Fatalf("resize event = %v", got)
	}
	width, height, err := host.Size()
	if err != nil || width != resized.Width || height != resized.Height {
		t.Fatalf("event and size disagree: %d, %d, %v", width, height, err)
	}
	if host.Send(input.Resize{}) {
		t.Fatal("generic Send bypassed resize validation")
	}
}
