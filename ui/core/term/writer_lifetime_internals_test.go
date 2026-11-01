package term

import (
	"errors"
	"runtime"
	"testing"
	"testing/synctest"
	"time"
)

type exitingDestination struct{ release <-chan struct{} }

func (d exitingDestination) Write([]byte) (int, error) {
	<-d.release
	runtime.Goexit()
	return 0, nil
}

func TestWriterExitSettlesAcceptedFramesAndEndsAdmission(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		release := make(chan struct{})
		writer := NewWriter(WriterConfig{Destination: exitingDestination{release: release}})
		t.Cleanup(func() { _ = writer.Close() })
		first, _ := writer.Queue([]byte("partial frame"))
		last, _ := writer.Queue([]byte("queued frame"))
		if first == 0 || last <= first {
			t.Fatal("writer did not accept the test frames")
		}
		close(release)
		synctest.Wait()

		if err := writer.Err(); !errors.Is(err, errWriteInterrupted) {
			t.Errorf("interrupted write = %v, want the destination failure", err)
		}
		if err := writer.Drain(time.Second); err != nil {
			t.Errorf("accepted frames were not settled on worker exit: %v", err)
		}
		select {
		case <-writer.Changes():
		default:
			t.Error("worker exit did not notify its owner")
		}
		if got, _ := writer.Queue([]byte("after worker exit")); got != 0 {
			t.Errorf("queue accepted sequence %d without a live writer", got)
		}
		if got := writer.Queued(); got != last {
			t.Errorf("refusal advanced accepted watermark to %d, want %d", got, last)
		}
		if got := writer.Written(); got != 0 {
			t.Errorf("interrupted frames advanced delivery watermark to %d", got)
		}
		if err := writer.Err(); !errors.Is(err, errWriteInterrupted) {
			t.Errorf("refusal replaced the original interruption cause: %v", err)
		}
	})
}

type destinationPanic struct{ cause error }

func (d destinationPanic) Write([]byte) (int, error) { panic(d.cause) }

func TestDestinationPanicSettlesWriterWithoutKillingItsSession(t *testing.T) {
	cause := errors.New("broken terminal bridge")
	w := NewWriter(WriterConfig{Destination: destinationPanic{cause}})
	if _, err := w.Queue([]byte("frame")); err != nil {
		t.Fatal(err)
	}
	if err := w.Drain(time.Second); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(w.Err(), cause) || !errors.Is(w.Err(), errWriteInterrupted) || w.Written() != 0 {
		t.Fatalf("write=%d error=%v", w.Written(), w.Err())
	}
	_ = w.Close()
}

type partialRefusal struct{ calls int }

func (d *partialRefusal) Write([]byte) (int, error) {
	d.calls++
	return 1, ErrFrameRefused
}

func TestPartialRefusalCannotAuthorizeAnotherTerminalWrite(t *testing.T) {
	d := &partialRefusal{}
	w := NewWriter(WriterConfig{Destination: d})
	defer func() { _ = w.Close() }()
	if _, err := w.Queue([]byte("long frame")); err != nil {
		t.Fatal(err)
	}
	if err := w.Drain(time.Second); err != nil {
		t.Fatal(err)
	}
	if seq, err := w.QueueControl([]byte("restore")); seq != 0 || err == nil {
		t.Fatalf("control frame admitted after ambiguous prefix: seq=%d error=%v", seq, err)
	}
	if d.calls != 1 || w.Written() != 0 {
		t.Fatalf("writes=%d watermark=%d", d.calls, w.Written())
	}
}
