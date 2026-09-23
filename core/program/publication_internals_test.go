package program

import (
	"errors"
	"testing"
	"time"
)

// stuckWriter answers every frame with the same sequence, which is the one thing a
// [FrameWriter] promises not to do.
type stuckWriter struct{ accepted int }

func (w *stuckWriter) Queue([]byte) uint64     { w.accepted++; return 1 }
func (*stuckWriter) Changes() <-chan struct{}  { return nil }
func (*stuckWriter) Written() uint64           { return 0 }
func (*stuckWriter) Err() error                { return nil }
func (*stuckWriter) Drain(time.Duration) error { return nil }

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
