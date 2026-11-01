package uitest

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/program"
	"github.com/Tangerg/oolong/ui/core/term"
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
	host := &Host{input: newEventSource(), writer: term.NewWriter(term.WriterConfig{Destination: output})}
	defer func() { _ = host.Close() }()
	defer release()
	if _, err := host.writer.Queue([]byte("frame")); err != nil {
		t.Fatal(err)
	}
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
	host := &Host{input: newEventSource(), writer: term.NewWriter(term.WriterConfig{Destination: failedOutput{err: want}})}
	if _, err := host.writer.Queue([]byte("frame")); err != nil {
		t.Fatal(err)
	}
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

func TestHostRejectsMutableOrEmptyInputBeforeQueueing(t *testing.T) {
	h := New(t, Config{Width: 8, Height: 3})
	for _, event := range []input.Event{(*input.Key)(nil), &input.Resize{Width: 20, Height: 10}, nil} {
		if h.Send(event) {
			t.Errorf("accepted %T", event)
		}
	}
	if w, h, err := h.Size(); err != nil || w != 8 || h != 3 {
		t.Fatalf("invalid event changed reported size: %d,%d %v", w, h, err)
	}
}

func TestRepaintCapturesTheAcknowledgedFrame(t *testing.T) {
	host := New(t, Config{Width: 8, Height: 3})
	go func() {
		reply := <-host.Repaints()
		// A later resize can already be queued, and an unrelated frame can have
		// arrived. Neither belongs to the geometry or last write acknowledged here.
		host.Resize(9, 4)
		_, _ = host.out.Write([]byte("prior"))
		_, _ = host.out.Write([]byte("requested"))
		reply(7, 2, nil)
		_, _ = host.out.Write([]byte("later"))
	}()
	snapshot, err := host.Repaint(t.Context())
	if err != nil || snapshot != (FrameSnapshot{Width: 7, Height: 2, Frame: "requested", Frames: "priorrequested"}) {
		t.Fatalf("settled snapshot: %+v, %v", snapshot, err)
	}
}

func TestRepaintReportsOwnerFailureAndAbandonment(t *testing.T) {
	host := New(t, Config{Width: 8, Height: 3})
	want := errors.New("frame was refused")
	go func() { (<-host.Repaints())(0, 0, want) }()
	if _, err := host.Repaint(t.Context()); !errors.Is(err, want) {
		t.Fatalf("delivery error = %v", err)
	}
	ctx, cancel := context.WithCancelCause(t.Context())
	go func() { <-host.Repaints(); cancel(want) }()
	if _, err := host.Repaint(ctx); !errors.Is(err, want) {
		t.Fatalf("cancelled wait = %v", err)
	}
	go func() { <-host.Repaints(); _ = host.Close() }()
	if _, err := host.Repaint(t.Context()); !errors.Is(err, program.ErrStopped) {
		t.Fatalf("closed host = %v", err)
	}
}
