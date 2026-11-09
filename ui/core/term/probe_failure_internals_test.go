package term

import (
	"errors"
	"io"
	"testing"
	"testing/synctest"
	"time"

	"github.com/Tangerg/oolong/ui/core/input"
)

type failingProbeWriter struct {
	written                         int
	writeErr, deadlineErr, resetErr error
	attempted, reset                bool
}

func (w *failingProbeWriter) SetWriteDeadline(deadline time.Time) error {
	if deadline.IsZero() {
		w.reset = true
		return w.resetErr
	}
	return w.deadlineErr
}

func (w *failingProbeWriter) WriteString(string) (int, error) {
	w.attempted = true
	return w.written, w.writeErr
}

func TestProbePropagatesTransportFailure(t *testing.T) {
	failure := errors.New("transport failed")
	for _, tc := range []struct {
		name   string
		writer failingProbeWriter
		want   error
	}{
		{"zero write", failingProbeWriter{writeErr: failure}, failure},
		{"partial control string", failingProbeWriter{written: len("\x1b]11;"), writeErr: failure}, failure},
		{"short write", failingProbeWriter{written: len("\x1b]11;")}, io.ErrShortWrite},
		{"deadline refused", failingProbeWriter{deadlineErr: failure}, failure},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := probe{out: &tc.writer, stream: &input.Stream{}, deadline: time.Now().Add(answerGrace)}
			_, err := p.run()
			if !errors.Is(err, tc.want) {
				t.Fatalf("probe error = %v, want %v", err, tc.want)
			}
			if tc.writer.deadlineErr == nil && !tc.writer.reset {
				t.Fatal("probe retained its output deadline")
			}
			if tc.writer.deadlineErr != nil && tc.writer.attempted {
				t.Fatal("probe wrote without a bounded transport")
			}
		})
	}
}

func TestProbeRetainsWriteAndDeadlineResetFailures(t *testing.T) {
	writeErr, resetErr := errors.New("partial write"), errors.New("deadline reset")
	out := &failingProbeWriter{written: 1, writeErr: writeErr, resetErr: resetErr}
	p := probe{out: out, stream: &input.Stream{}, deadline: time.Now().Add(answerGrace)}
	_, err := p.run()
	if !errors.Is(err, writeErr) || !errors.Is(err, resetErr) {
		t.Fatalf("probe error = %v, want both failures", err)
	}
}

func TestProbeSilenceRemainsAnUnknownCapability(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		p := probe{out: answerWriter(func(string) {}), stream: &input.Stream{}, deadline: time.Now().Add(answerGrace)}
		got, err := p.run()
		if err != nil || got.hasAttrs || got.hasName || got.hasFg || got.hasBg {
			t.Fatalf("silent terminal = %+v, %v", got, err)
		}
	})
}
