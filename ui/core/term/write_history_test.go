package term_test

import (
	"io"
	"testing"
	"testing/synctest"
	"time"

	"github.com/Tangerg/oolong/ui/core/term"
)

func TestWrittenAtRejectsEqualCompletionTimestampsInEitherOrder(t *testing.T) {
	for _, beforeWrite := range []bool{false, true} {
		name := "input after write"
		if beforeWrite {
			name = "input before write"
		}
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				writer := term.NewWriter(term.WriterConfig{Destination: io.Discard})
				defer func() {
					if err := writer.Close(); err != nil {
						t.Error(err)
					}
				}()
				if sequence, known := writer.WrittenAt(time.Now()); known || sequence != 0 {
					t.Fatalf("creation timestamp established writer lifetime: (%d, %t)", sequence, known)
				}
				time.Sleep(time.Nanosecond)
				if _, err := writer.Queue([]byte("first")); err != nil {
					t.Fatal(err)
				}
				if err := writer.Drain(time.Second); err != nil {
					t.Fatal(err)
				}
				time.Sleep(time.Nanosecond)
				arrival := time.Now()
				if _, err := writer.Queue([]byte("second")); err != nil {
					t.Fatal(err)
				}
				if err := writer.Drain(time.Second); err != nil {
					t.Fatal(err)
				}
				if !beforeWrite {
					arrival = time.Now()
				}
				if sequence, known := writer.WrittenAt(arrival); known || sequence != 0 {
					t.Fatalf("equal completion timestamp certified sequence (%d, %t)", sequence, known)
				}
				time.Sleep(time.Nanosecond)
				if sequence, known := writer.WrittenAt(time.Now()); !known || sequence != 2 {
					t.Fatalf("distinct later timestamp = (%d, %t), want (2, true)", sequence, known)
				}
			})
		})
	}
}

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

func TestWrittenNowObservesCompletionWithoutClockAdvance(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		destination := newBlocker()
		writer := newTestWriter(t, destination)
		defer destination.releaseAll()
		instant := time.Now()
		if sequence, settled := writer.WrittenNow(); sequence != 0 || !settled {
			t.Fatalf("new writer prefix = (%d, %t)", sequence, settled)
		}
		sequence, err := writer.Queue([]byte("frame"))
		if err != nil {
			t.Fatal(err)
		}
		synctest.Wait()
		if got, settled := writer.WrittenNow(); got != 0 || settled {
			t.Fatalf("incomplete frame certified prefix (%d, %t)", got, settled)
		}
		destination.release <- struct{}{}
		if err := writer.Drain(time.Second); err != nil {
			t.Fatal(err)
		}
		if got, settled := writer.WrittenNow(); got != sequence || !settled {
			t.Fatalf("completed frame certified prefix (%d, %t), want (%d, true)", got, settled, sequence)
		}
		if !time.Now().Equal(instant) {
			t.Fatal("test advanced the clock before observing completion")
		}
		if _, settled := writer.WrittenAt(instant); settled {
			t.Fatal("current observation changed physical arrival evidence")
		}
	})
}
