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
		writer := NewWriter(exitingDestination{release: release})
		t.Cleanup(func() { _ = writer.Close() })
		first := writer.Queue([]byte("partial frame"))
		last := writer.Queue([]byte("queued frame"))
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
		if got := writer.Queue([]byte("after worker exit")); got != 0 {
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
