package term

import (
	"testing"
	"time"
)

func TestWriteHistoryProvesOnlySettledIntervals(t *testing.T) {
	start := time.Unix(100, 0)
	at := func(seconds int) time.Time { return start.Add(time.Duration(seconds) * time.Second) }
	history := writeHistory{opened: start, limit: 2}
	history.begin(1, at(1))
	history.complete(1, at(2))
	history.begin(2, at(4))
	for _, check := range []struct {
		at       time.Time
		sequence uint64
		known    bool
	}{
		{at(-1), 0, false},
		{at(0), 0, false},
		{at(0).Add(time.Nanosecond), 0, true},
		{at(1), 0, false},
		{at(2), 0, false},
		{at(3), 1, true},
		{at(4), 0, false},
		{at(5), 0, false},
		{at(11), 0, false},
	} {
		sequence, known := history.at(check.at, at(10))
		if sequence != check.sequence || known != check.known {
			t.Errorf("at %v: got (%d, %t), want (%d, %t)", check.at, sequence, known, check.sequence, check.known)
		}
	}
	history.complete(2, at(6))
	if sequence, known := history.at(at(6), at(10)); known || sequence != 0 {
		t.Fatalf("equal completion timestamp proved causal order: (%d, %t)", sequence, known)
	}
	if sequence, known := history.at(at(7), at(10)); sequence != 2 || !known {
		t.Fatalf("second completion = (%d, %t)", sequence, known)
	}
	history.begin(3, at(8))
	history.complete(3, at(9))
	if sequence, known := history.at(at(3), at(10)); known || sequence != 0 {
		t.Fatalf("evicted completion was replaced with newer evidence: (%d, %t)", sequence, known)
	}
	if sequence, known := history.at(at(7), at(10)); sequence != 2 || !known {
		t.Fatalf("retained historical completion = (%d, %t)", sequence, known)
	}
	if history.latest() != 3 || len(history.records) != 2 {
		t.Fatalf("history=%+v", history.records)
	}
}

func TestWriteHistoryCannotOrderWritesAtTheSameInstant(t *testing.T) {
	start := time.Unix(100, 0)
	history := writeHistory{opened: start, limit: 3}
	history.begin(1, start.Add(time.Second))
	history.complete(1, start.Add(2*time.Second))
	boundary := start.Add(4 * time.Second)
	for _, sequence := range []uint64{2, 3} {
		history.begin(sequence, boundary)
		history.complete(sequence, boundary)
	}
	now := boundary.Add(time.Second)
	if sequence, known := history.at(boundary, now); known || sequence != 0 {
		t.Fatalf("equal boundaries selected a completed prefix: (%d, %t)", sequence, known)
	}
	if sequence, known := history.at(boundary.Add(-time.Nanosecond), now); !known || sequence != 1 {
		t.Fatalf("earlier settled prefix = (%d, %t), want (1, true)", sequence, known)
	}
	if sequence, known := history.at(boundary.Add(time.Nanosecond), now); !known || sequence != 3 {
		t.Fatalf("later settled prefix = (%d, %t), want (3, true)", sequence, known)
	}
}

func TestWriteHistoryCannotCertifyPastADeliveryFailure(t *testing.T) {
	start := time.Unix(100, 0)
	history := writeHistory{opened: start, limit: 3}
	history.begin(1, start.Add(time.Second))
	history.complete(1, start.Add(2*time.Second))
	history.begin(2, start.Add(4*time.Second))
	history.begin(3, start.Add(6*time.Second))
	history.complete(3, start.Add(7*time.Second))
	if history.latest() != 1 {
		t.Fatal("later control output vouched for an earlier failed frame")
	}
	if _, known := history.at(start.Add(8*time.Second), start.Add(9*time.Second)); known {
		t.Fatal("failed delivery authorized coordinate input")
	}
	if sequence, known := history.at(start.Add(3*time.Second), start.Add(9*time.Second)); sequence != 1 || !known {
		t.Fatalf("failure erased earlier valid evidence: (%d, %t)", sequence, known)
	}
}
