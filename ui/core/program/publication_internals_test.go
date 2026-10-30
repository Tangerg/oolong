package program

import (
	"errors"
	"testing"
	"time"
)

// stuckWriter answers every frame with the same sequence, which is the one thing a
// [FrameWriter] promises not to do.
type stuckWriter struct{ accepted int }

func (w *stuckWriter) Queue([]byte) (uint64, error) { w.accepted++; return 1, nil }
func (*stuckWriter) Changes() <-chan struct{}       { return nil }
func (*stuckWriter) Written() uint64                { return 0 }
func (*stuckWriter) Err() error                     { return nil }
func (*stuckWriter) Drain(time.Duration) error      { return nil }

type refusingWriter struct {
	stuckWriter
	cause error
}

func (w *refusingWriter) Queue([]byte) (uint64, error) { w.accepted++; return 0, w.cause }
func (w *refusingWriter) Err() error                   { return w.cause }

func TestRefusedFrameReportsTheAdmissionCauseAndEndsPublication(t *testing.T) {
	cause := errors.New("transport closed")
	w := &refusingWriter{cause: cause}
	p := &program{writer: w}
	if sequence, err := p.queue([]byte("refused")); sequence != 0 || !errors.Is(err, cause) {
		t.Fatalf("refusal lost its cause: sequence=%d error=%v", sequence, err)
	}
	if _, err := p.queue([]byte("late")); !errors.Is(err, ErrDisplayFailed) || w.accepted != 1 || p.queued != 0 {
		t.Fatalf("refusal advanced publication: calls=%d queued=%d error=%v", w.accepted, p.queued, err)
	}
}

// TestNoFrameIsPublishedAfterTheSequenceContractBreaks holds the publication edge to
// what the program says about itself.
//
// A host supplies its own writer, so the sequence contract is a promise this program
// checks rather than one it makes. Once the check has failed, the position of the
// terminal's byte stream is no longer known, and handing that stream another frame
// is the one thing that cannot be undone.
func TestNoFrameIsPublishedAfterTheSequenceContractBreaks(t *testing.T) {
	writer := &stuckWriter{}
	p := &program{writer: writer}

	if _, err := p.queue([]byte("first")); err != nil {
		t.Fatalf("the first frame was refused: %v", err)
	}
	if _, err := p.queue([]byte("second")); !errors.Is(err, ErrInvalidFrameSequence) {
		t.Fatalf("a repeated sequence gave %v, want ErrInvalidFrameSequence", err)
	}
	accepted := writer.accepted

	if _, err := p.queue([]byte("third")); !errors.Is(err, ErrDisplayFailed) {
		t.Fatalf("publishing after the failure gave %v, want ErrDisplayFailed", err)
	}
	if writer.accepted != accepted {
		t.Fatalf("%d frames reached the writer after it was known unusable", writer.accepted-accepted)
	}
}
