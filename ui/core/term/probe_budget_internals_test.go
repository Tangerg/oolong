package term

import (
	"bytes"
	"encoding/base64"
	"errors"
	"image/png"
	"io"
	"strconv"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/Tangerg/oolong/ui/core/graphics"
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

type noDeadlineWriter struct{ attempted bool }

func (*noDeadlineWriter) SetWriteDeadline(time.Time) error { return errors.ErrUnsupported }
func (w *noDeadlineWriter) WriteString(string) (int, error) {
	w.attempted = true
	return 0, io.ErrClosedPipe
}

func TestProbeCannotAttemptAnUnboundedWrite(t *testing.T) {
	out := &noDeadlineWriter{}
	pr := &probe{out: out, deadline: time.Now().Add(answerGrace)}
	pr.run()
	if out.attempted {
		t.Fatal("optional probe wrote after the transport refused its write deadline")
	}
}

type answerWriter func(string)

func (answerWriter) SetWriteDeadline(time.Time) error { return nil }

func (w answerWriter) WriteString(query string) (int, error) {
	w(query)
	return len(query), nil
}

func TestDirectGraphicsQueryOwnsTheStartupDecision(t *testing.T) {
	pixel, err := base64.StdEncoding.DecodeString(queryPixelPNG)
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(pixel))
	if err != nil || img.Bounds().Dx() != 1 || img.Bounds().Dy() != 1 {
		t.Fatal("invalid 1×1 probe PNG:", err)
	}
	const marker = "\x1b[?62;4c"
	for _, tc := range []struct {
		name, reply string
		hint, want  graphics.Protocol
		early       int
	}{
		{"direct OK", "KEY;OK" + marker, graphics.None, graphics.Kitty, 0},
		{"direct error", "KEY;EINVAL:unsupported", graphics.Kitty, graphics.None, 0},
		{"missing before DA", marker, graphics.Kitty, graphics.Sixel, 0},
		{"unanswered exchange", "", graphics.Kitty, graphics.Kitty, 0},
		{"wrong identity", "other;OK" + marker, graphics.None, graphics.Sixel, 1},
		{"late answer", marker + "KEY;OK", graphics.None, graphics.Sixel, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := make(chan readResult, 1)
			w := answerWriter(func(query string) {
				_, tail, ok := strings.Cut(query, "\x1b_Gi=")
				id, suffix, found := strings.Cut(tail, ",")
				value, err := strconv.ParseUint(id, 10, 32)
				if !ok || !found || err != nil || value == 0 || suffix != "a=q,t=d,f=100;"+queryPixelPNG+"\x1b\\\x1b[c" {
					t.Fatalf("unexpected query %q", query)
				}
				reply := strings.NewReplacer("KEY;OK", "\x1b_Gi="+id+";OK\x1b\\",
					"other;OK", "\x1b_Gi=0;OK\x1b\\", "KEY;EINVAL:unsupported", "\x1b_Gi="+id+";EINVAL:unsupported\x1b\\").Replace(tc.reply)
				raw <- readResult{data: []byte(reply), err: io.EOF}
			})
			p := probe{raw: raw, out: w, stream: &input.Stream{}, deadline: time.Now().Add(time.Second)}
			got := p.run()
			if actual := got.graphics(tc.hint); actual != tc.want || len(p.early) != tc.early {
				t.Fatalf("graphics = %v, early = %#v, want %v and %d events", actual, p.early, tc.want, tc.early)
			}
		})
	}
}

func TestProbeYieldsToInputWithoutLosingPendingFraming(t *testing.T) {
	const chunkSize = 1024
	const count = probeInputBudget - len("\x1b[")
	at := time.Now()
	raw := make(chan readResult, probeInputBudget/chunkSize+1)
	rawBytes := strings.Repeat("x", count) + "\x1b["
	for i := 0; i < len(rawBytes); i += chunkSize {
		raw <- readResult{data: []byte(rawBytes[i:min(len(rawBytes), i+chunkSize)]), at: at}
	}
	raw <- readResult{data: []byte("A\x1b[?62c"), at: at, err: io.EOF}
	stream := &input.Stream{}
	pr := probe{raw: raw, out: answerWriter(func(string) {}), stream: stream, deadline: at.Add(time.Second)}
	if got := pr.run(); got.hasAttrs || len(pr.early) != count {
		t.Fatalf("probe consumed the next read after %d keys: attrs=%v early=%d", count, got.hasAttrs, len(pr.early))
	}
	out := make(chan input.Event, count+2)
	p := pump{raw: raw, end: pr.end, stream: stream, early: pr.early, out: out}
	if err := p.run(); err != nil {
		t.Fatal(err)
	}
	if len(out) != count+2 {
		t.Fatalf("pump delivered %d events, want %d", len(out), count+2)
	}
	for range count {
		if key, ok := (<-out).(input.Key); !ok || !key.IsRune('x', 0) {
			t.Fatal("startup input was lost or reordered")
		}
	}
	if key, ok := (<-out).(input.Key); !ok || !key.Is(input.Up, 0) {
		t.Fatal("handoff lost the split key")
	}
	if a, ok := (<-out).(input.DeviceAttributes); !ok || a.Class != 62 {
		t.Fatal("the session lost the late probe answer")
	}
}
