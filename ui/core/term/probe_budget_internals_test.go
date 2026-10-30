package term

import (
	"errors"
	"io"
	"testing"
	"testing/synctest"
	"time"

	"github.com/Tangerg/oolong/ui/core/input"
)

// blockedWriter is a terminal that will not take the query: the write parks until
// its own deadline, which is what a terminal under output backpressure does.
type blockedWriter struct {
	deadline time.Time
	armed    bool
}

func (w *blockedWriter) SetWriteDeadline(deadline time.Time) error {
	w.deadline = deadline
	w.armed = w.armed || !deadline.IsZero()
	return nil
}

func (w *blockedWriter) WriteString(string) (int, error) {
	if w.deadline.IsZero() {
		// No deadline means no way to stop waiting to be allowed to write.
		select {}
	}
	time.Sleep(time.Until(w.deadline))
	return 0, deadlineError{}
}

type deadlineError struct{}

func (deadlineError) Error() string   { return "term: write deadline exceeded" }
func (deadlineError) Timeout() bool   { return true }
func (deadlineError) Temporary() bool { return true }

func TestProbeBudgetCoversTheQueryWrite(t *testing.T) {
	// A write with no deadline never reaches the wait that bounds the rest, and the
	// caller cannot intervene: Open has not returned the terminal whose Close would
	// give raw mode back.
	synctest.Test(t, func(t *testing.T) {
		deadline := time.Now().Add(answerGrace)
		out := &blockedWriter{}
		p := &probe{
			raw:      make(chan readResult),
			out:      out,
			stream:   input.NewStream(input.StreamConfig{}),
			deadline: deadline,
		}
		done := make(chan struct{})
		go func() { p.run(); close(done) }()

		time.Sleep(3 * answerGrace)
		synctest.Wait()
		select {
		case <-done:
		default:
			t.Fatal("the probe never returned: its budget did not cover the query write")
		}
		if !out.armed {
			t.Fatal("the probe wrote without holding the transport to its budget")
		}
	})
}

func TestProbeBudgetIsOneBudgetForAskingAndAnswering(t *testing.T) {
	// Spending most of it on the write leaves the rest for the answer, rather than
	// starting a fresh full wait once the write is through.
	synctest.Test(t, func(t *testing.T) {
		start := time.Now()
		deadline := start.Add(answerGrace)
		p := &probe{
			raw:      make(chan readResult),
			out:      slowWriter{until: start.Add(answerGrace * 3 / 4)},
			stream:   input.NewStream(input.StreamConfig{}),
			deadline: deadline,
		}
		done := make(chan struct{})
		go func() { p.run(); close(done) }()

		time.Sleep(answerGrace)
		synctest.Wait()
		select {
		case <-done:
		default:
			t.Fatal("the probe waited a fresh grace after the write instead of sharing one budget")
		}
	})
}

type slowWriter struct{ until time.Time }

func (slowWriter) SetWriteDeadline(time.Time) error { return nil }

func (w slowWriter) WriteString(s string) (int, error) {
	time.Sleep(time.Until(w.until))
	return len(s), nil
}

func TestProbeHandsInputEndToThePump(t *testing.T) {
	for _, cause := range []error{io.EOF, errors.New("read failed")} {
		synctest.Test(t, func(t *testing.T) {
			at := time.Now()
			raw := make(chan readResult, 1)
			raw <- readResult{data: []byte("x\x1b"), at: at, err: cause}
			stream := &input.Stream{}
			pr := &probe{raw: raw, out: slowWriter{until: at}, stream: stream, deadline: at.Add(answerGrace)}
			pr.run()
			if pr.end == nil || !errors.Is(pr.end.err, cause) {
				t.Fatal("probe lost the final read")
			}
			out := make(chan input.Event, 2)
			p := &pump{raw: raw, end: pr.end, stream: stream, early: pr.early, out: out}
			err := p.run()
			if errors.Is(cause, io.EOF) {
				if err != nil {
					t.Fatal(err)
				}
			} else if !errors.Is(err, cause) {
				t.Fatal("pump lost the read failure")
			}
			first := (<-out).(input.Key)
			last := (<-out).(input.Key)
			if !first.IsRune('x', 0) || last.Code != input.Esc || !last.At.Equal(at) {
				t.Fatalf("handover changed input: %+v, %+v", first, last)
			}
		})
	}
}
