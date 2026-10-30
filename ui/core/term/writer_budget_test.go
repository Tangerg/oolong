package term_test

import (
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/oolong/ui/core/term"
)

func TestWriterBudgetIncludesInFlightAndReservesTeardown(t *testing.T) {
	destination := newBlocker()
	writer := term.NewWriter(term.WriterConfig{Destination: destination, MaxFrames: 2, MaxFrameBytes: 4, MaxPendingBytes: 5})
	t.Cleanup(func() { _ = writer.Close() })
	if _, err := writer.Queue([]byte("abc")); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Queue([]byte("de")); err != nil {
		t.Fatal(err)
	}
	if sequence, err := writer.Queue([]byte("f")); sequence != 0 || !errors.Is(err, term.ErrQueueFull) {
		t.Fatalf("capacity sequence=%d error=%v", sequence, err)
	}
	if sequence, err := writer.Queue([]byte("large")); sequence != 0 || !errors.Is(err, term.ErrPayloadTooLarge) {
		t.Fatalf("payload sequence=%d error=%v", sequence, err)
	}
	if writer.Queued() != 2 || writer.Err() != nil {
		t.Fatal("recoverable refusal poisoned writer progress")
	}
	writer.CloseAdmission()
	sequence, err := writer.QueueControl([]byte("restore"))
	if err != nil || sequence != 3 {
		t.Fatalf("reserved control=%d, %v", sequence, err)
	}
	destination.releaseAll()
	if err := writer.Drain(time.Second); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"abc", "de", "restore"} {
		if got := string(<-destination.writes); got != want {
			t.Fatalf("output=%q want=%q", got, want)
		}
	}
	if writer.Written() != sequence {
		t.Fatal("control output overtook accepted data")
	}
}
