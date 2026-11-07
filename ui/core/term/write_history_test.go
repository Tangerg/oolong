package term_test

import (
	"testing"
	"testing/synctest"
	"time"

	"github.com/Tangerg/oolong/ui/core/term"
)

func TestWrittenAtRetainsOnlyProvableBoundedHistory(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		destination := newBlocker()
		writer := term.NewWriter(term.WriterConfig{Destination: destination, CompletionHistory: 2})
		defer func() {
			destination.releaseAll()
			if err := writer.Close(); err != nil {
				t.Error(err)
			}
		}()
		type completion struct {
			at       time.Time
			sequence uint64
		}
		var completed []completion
		for index, payload := range []string{"first", "second", "third"} {
			time.Sleep(time.Millisecond)
			sequence, err := writer.Queue([]byte(payload))
			if err != nil {
				t.Fatal(err)
			}
			synctest.Wait()
			time.Sleep(time.Millisecond)
			during := time.Now()
			if got, known := writer.WrittenAt(during); known {
				t.Fatalf("blocked frame %d certified sequence %d", sequence, got)
			}
			if index == 1 {
				first := completed[0]
				if got, known := writer.WrittenAt(first.at); !known || got != first.sequence {
					t.Fatalf("later write changed earlier evidence: (%d, %t), want (%d, true)", got, known, first.sequence)
				}
			}

			// Distinct virtual instants keep evidence inside a write separate from its completion.
			time.Sleep(time.Millisecond)
			destination.release <- struct{}{}
			if err := writer.Drain(time.Second); err != nil {
				t.Fatal(err)
			}
			time.Sleep(time.Millisecond)
			after := time.Now()
			if got, known := writer.WrittenAt(after); !known || got != sequence {
				t.Fatalf("completed frame evidence = (%d, %t), want (%d, true)", got, known, sequence)
			}
			if got, known := writer.WrittenAt(during); known {
				t.Fatalf("completion retroactively certified in-flight time as sequence %d", got)
			}
			completed = append(completed, completion{at: after, sequence: sequence})
			if index == 1 {
				first := completed[0]
				if got, known := writer.WrittenAt(first.at); !known || got != first.sequence {
					t.Fatalf("new completion retargeted old time: (%d, %t), want (%d, true)", got, known, first.sequence)
				}
			}
		}
		if got, known := writer.WrittenAt(completed[0].at); known {
			t.Fatalf("expired history was replaced by sequence %d", got)
		}
		second := completed[1]
		if got, known := writer.WrittenAt(second.at); !known || got != second.sequence {
			t.Fatalf("retained history = (%d, %t), want (%d, true)", got, known, second.sequence)
		}
	})
}
