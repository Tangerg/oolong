package term_test

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Tangerg/oolong/core/term"
)

// recorder collects everything written to it.
type recorder struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (r *recorder) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.b.Write(p)
}

func (r *recorder) String() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.b.String()
}

// blocker holds writes until it is released, standing in for a terminal that has
// stopped accepting bytes.
type blocker struct {
	release chan struct{}
	writes  chan []byte
}

func newBlocker() *blocker {
	return &blocker{release: make(chan struct{}), writes: make(chan []byte, 16)}
}

func (b *blocker) Write(p []byte) (int, error) {
	<-b.release
	b.writes <- append([]byte(nil), p...)
	return len(p), nil
}

// failing fails after letting a number of writes through.
type failing struct {
	mu    sync.Mutex
	ok    int
	calls int
}

var errBroken = errors.New("terminal went away")

func (f *failing) Write(p []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.ok > 0 {
		f.ok--
		return len(p), nil
	}
	return 0, errBroken
}

// short writes one byte at a time, which is what a terminal is allowed to do.
type short struct{ b bytes.Buffer }

func (s *short) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	return s.b.Write(p[:1])
}

// partial accepts a visible prefix and fails in the same call.
type partial struct {
	b     bytes.Buffer
	calls int
}

func (p *partial) Write(frame []byte) (int, error) {
	p.calls++
	n := min(3, len(frame))
	_, _ = p.b.Write(frame[:n])
	return n, errBroken
}

func TestFramesReachTheTerminalInOrder(t *testing.T) {
	dst := &recorder{}
	w := term.NewWriter(dst)

	var last uint64
	for _, frame := range []string{"one", "two", "three"} {
		seq := w.Queue([]byte(frame))
		if seq != last+1 {
			t.Fatalf("sequence = %d, want %d", seq, last+1)
		}
		last = seq
	}
	if err := w.Drain(time.Second); err != nil {
		t.Fatalf("frames never drained: %v", err)
	}
	if got := dst.String(); got != "onetwothree" {
		t.Fatalf("terminal received %q", got)
	}
	if got := w.Written(); got != 3 {
		t.Fatalf("watermark = %d, want 3", got)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestQueueDoesNotWaitForTheTerminal(t *testing.T) {
	dst := newBlocker()
	w := term.NewWriter(dst)
	defer dst.releaseAll()

	// The whole reason this type exists: a terminal that is not accepting bytes must
	// not stop the loop that draws.
	done := make(chan uint64, 1)
	go func() {
		var last uint64
		for range 100 {
			last = w.Queue([]byte("frame"))
		}
		done <- last
	}()
	select {
	case seq := <-done:
		if seq != 100 {
			t.Fatalf("sequence = %d, want 100", seq)
		}
	case <-time.After(time.Second):
		t.Fatal("Queue blocked on a terminal that was not accepting bytes")
	}
	if got := w.Written(); got != 0 {
		t.Fatalf("watermark = %d, want nothing written yet", got)
	}
}

func (b *blocker) releaseAll() { close(b.release) }

func TestProgressWakesTheLoopWhenTheWatermarkMoves(t *testing.T) {
	dst := newBlocker()
	w := term.NewWriter(dst)

	seq := w.Queue([]byte("frame"))
	select {
	case <-w.Progress():
		t.Fatal("progress reported before anything was written")
	case <-time.After(20 * time.Millisecond):
	}

	dst.releaseAll()
	select {
	case <-w.Progress():
	case <-time.After(time.Second):
		t.Fatal("progress never reported")
	}
	if got := w.Written(); got != seq {
		t.Fatalf("watermark = %d, want %d", got, seq)
	}
}

func TestProgressCoalesces(t *testing.T) {
	// A loop that has not noticed the last advance does not need to be told twice,
	// and a bounded signal is what keeps a burst of frames from queueing wake-ups.
	//
	// This depends on the writer publishing a wake-up before it moves the watermark
	// Drain returns on: the other order leaves the last frame's wake-up in flight
	// after Drain has returned, and one more arrives just after the first is taken.
	// It failed that way once, on a machine that interleaved it differently from
	// eight hundred runs here.
	w := term.NewWriter(&recorder{})
	for range 5 {
		w.Queue([]byte("x"))
	}
	if err := w.Drain(time.Second); err != nil {
		t.Fatalf("frames never drained: %v", err)
	}
	<-w.Progress()
	select {
	case <-w.Progress():
		t.Fatal("a second wake-up was queued for the same unobserved advance")
	case <-time.After(20 * time.Millisecond):
	}
}

func TestShortWritesAreCompleted(t *testing.T) {
	dst := &short{}
	w := term.NewWriter(dst)
	w.Queue([]byte("hello"))
	if err := w.Drain(time.Second); err != nil {
		t.Fatalf("never drained: %v", err)
	}
	if got := dst.b.String(); got != "hello" {
		t.Fatalf("terminal received %q, want the whole frame", got)
	}
}

func TestAFailedWriteIsReportedAndStopsFurtherWrites(t *testing.T) {
	dst := &failing{ok: 1}
	w := term.NewWriter(dst)

	w.Queue([]byte("first"))
	w.Queue([]byte("second"))
	w.Queue([]byte("third"))
	if err := w.Drain(time.Second); err != nil {
		t.Fatalf("failed frames never settled: %v", err)
	}

	if err := w.Err(); !errors.Is(err, errBroken) {
		t.Fatalf("Err = %v, want the write failure", err)
	}
	// A terminal that has failed a write does not recover. Continuing would produce
	// the same error over and over, and a partly-written frame after it.
	if dst.calls > 2 {
		t.Fatalf("wrote %d times, want it to have stopped at the failure", dst.calls)
	}
	if got := w.Written(); got != 1 {
		t.Fatalf("watermark = %d, want only the frame that succeeded", got)
	}
}

func TestFramesQueuedAfterFailureAreRefusedAndAccountedFor(t *testing.T) {
	dst := &failing{}
	w := term.NewWriter(dst)
	w.Queue([]byte("doomed"))
	if err := w.Drain(time.Second); err != nil {
		t.Fatalf("failed frame never settled: %v", err)
	}

	for range 100 {
		w.Queue([]byte("too late"))
	}
	if err := w.Drain(time.Second); err != nil {
		t.Fatalf("refused frames were not accounted for: %v", err)
	}
	if dst.calls != 1 {
		t.Fatalf("terminal Write called %d times, want no calls after the first failure", dst.calls)
	}
	if err := w.Err(); !errors.Is(err, errBroken) {
		t.Fatalf("Err = %v, want original terminal failure", err)
	}
}

func TestDrainCountsFailedFramesAsAccountedFor(t *testing.T) {
	// A broken terminal must not be able to wedge a shutdown.
	w := term.NewWriter(&failing{})
	w.Queue([]byte("doomed"))
	if err := w.Drain(time.Second); err != nil {
		t.Fatalf("Drain waited for a frame that had already failed: %v", err)
	}
}

func TestAPartialWriteFailsTheFrameAndStopsLaterWrites(t *testing.T) {
	dst := &partial{}
	w := term.NewWriter(dst)
	w.Queue([]byte("first frame"))
	w.Queue([]byte("second frame"))
	if err := w.Drain(time.Second); err != nil {
		t.Fatalf("partially failed frames were not accounted for: %v", err)
	}
	if err := w.Err(); !errors.Is(err, errBroken) {
		t.Fatalf("Err = %v, want partial-write cause", err)
	}
	if got := dst.b.String(); got != "fir" {
		t.Fatalf("terminal received %q, want only the ambiguous prefix", got)
	}
	if dst.calls != 1 {
		t.Fatalf("terminal Write called %d times, want no writes after failure", dst.calls)
	}
	if got := w.Written(); got != 0 {
		t.Fatalf("written watermark = %d, want no complete frame", got)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestDrainGivesUpOnATerminalThatNeverAccepts(t *testing.T) {
	dst := newBlocker()
	w := term.NewWriter(dst)
	defer dst.releaseAll()

	w.Queue([]byte("frame"))
	if err := w.Drain(30 * time.Millisecond); !errors.Is(err, term.ErrDrainTimeout) {
		t.Fatalf("Drain error = %v, want ErrDrainTimeout", err)
	}
}

func TestCloseReportsAbandonedFrames(t *testing.T) {
	dst := newBlocker()
	w := term.NewWriter(dst)
	defer dst.releaseAll()

	w.Queue([]byte("first"))
	w.Queue([]byte("second"))

	start := time.Now()
	err := w.Close()
	if err == nil {
		t.Fatal("Close hid the frames it abandoned")
	}
	if !errors.Is(err, term.ErrClosed) {
		t.Fatalf("Close error = %v, want it to be recognisable as term.ErrClosed", err)
	}
	// A write already inside the terminal cannot be interrupted, so Close returns on
	// its grace period rather than waiting for one that never comes back.
	if elapsed := time.Since(start); elapsed > 2*term.DrainGrace {
		t.Fatalf("Close took %v, want it bounded by the grace period", elapsed)
	}
}

func TestCloseIsIdempotent(t *testing.T) {
	w := term.NewWriter(&recorder{})
	w.Queue([]byte("frame"))
	first := w.Close()
	second := w.Close()
	if first != nil || second != nil {
		t.Fatalf("Close returned %v then %v, want neither", first, second)
	}
}

func TestQueueAfterCloseIsRefusedRatherThanFatal(t *testing.T) {
	w := term.NewWriter(&recorder{})
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	// Shutting down while something still wanted to draw is ordinary, and must not
	// take the process down with it.
	seq := w.Queue([]byte("too late"))
	if seq == 0 {
		t.Fatal("a refused frame got no sequence")
	}
	if err := w.Drain(time.Second); err != nil {
		t.Fatalf("a refused frame was never accounted for: %v", err)
	}
}

func TestARefusedFrameCannotOvertakeABlockedWriteInDrain(t *testing.T) {
	dst := newBlocker()
	w := term.NewWriter(dst)
	w.Queue([]byte("blocked"))
	if err := w.Close(); !errors.Is(err, term.ErrClosed) {
		t.Fatalf("Close error = %v, want term.ErrClosed", err)
	}

	// This frame completes immediately because the writer is closed. Its higher
	// sequence must not move the processed watermark past the older write that is
	// still inside the terminal.
	w.Queue([]byte("too late"))
	if err := w.Drain(30 * time.Millisecond); !errors.Is(err, term.ErrDrainTimeout) {
		t.Fatalf("Drain error = %v, want ErrDrainTimeout while the older write is blocked", err)
	}

	dst.releaseAll()
	if err := w.Drain(time.Second); err != nil {
		t.Fatalf("Drain did not advance after the older write settled: %v", err)
	}
}

func TestManyProducersKeepSequenceAndWriteOrderTogether(t *testing.T) {
	// Sequence order and write order have to agree, or the watermark would release a
	// frame that has not landed.
	dst := &recorder{}
	w := term.NewWriter(dst)
	const frames = 50
	type queued struct {
		seq  uint64
		data string
	}
	start := make(chan struct{})
	results := make(chan queued, frames)
	var wg sync.WaitGroup
	for i := range frames {
		data := fmt.Sprintf("%02d", i)
		wg.Go(func() {
			<-start
			results <- queued{seq: w.Queue([]byte(data)), data: data}
		})
	}
	close(start)
	wg.Wait()
	close(results)
	if err := w.Drain(2 * time.Second); err != nil {
		t.Fatalf("never drained: %v", err)
	}
	if got := w.Written(); got != frames {
		t.Fatalf("watermark = %d, want %d", got, frames)
	}
	ordered := make([]string, frames)
	for result := range results {
		ordered[result.seq-1] = result.data
	}
	if got, want := dst.String(), strings.Join(ordered, ""); got != want {
		t.Fatalf("terminal received %q, want sequence order %q", got, want)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestQueueAndCloseAreSafeTogether(t *testing.T) {
	for range 100 {
		w := term.NewWriter(&recorder{})
		start := make(chan struct{})
		var wg sync.WaitGroup
		for range 20 {
			wg.Go(func() {
				<-start
				w.Queue([]byte("frame"))
			})
		}
		wg.Go(func() {
			<-start
			_ = w.Close()
		})
		close(start)
		wg.Wait()
		if err := w.Drain(time.Second); err != nil {
			t.Fatalf("frames queued around Close were not accounted for: %v", err)
		}
	}
}

func TestDrainDoesNotTakeTheLoopsWakeUp(t *testing.T) {
	// Two things wait for the watermark: the loop, through Progress, and Drain.
	// Progress holds one wake-up and a taken one is gone, so a Drain that waited on
	// it would leave the loop with nothing to wake on — and would do so only on a
	// machine slow enough for Drain to get there first, which is how this hid.
	dst := newBlocker()
	w := term.NewWriter(dst)
	seq := w.Queue([]byte("frame"))

	// Drain is waiting before anything has been written, which is the ordering the
	// fast path never takes.
	drained := make(chan error, 1)
	go func() { drained <- w.Drain(2 * time.Second) }()
	time.Sleep(20 * time.Millisecond)
	dst.releaseAll()

	if err := <-drained; err != nil {
		t.Fatalf("never drained: %v", err)
	}
	select {
	case <-w.Progress():
	case <-time.After(time.Second):
		t.Fatal("the loop was never woken: Drain took the wake-up it was owed")
	}
	if got := w.Written(); got != seq {
		t.Fatalf("watermark = %d, want %d", got, seq)
	}
}

func TestQueuedCountsWhatWasHandedOver(t *testing.T) {
	// The sequence is reserved before the goroutine can see the frame, so the count
	// already accounts for it by the time Queue returns.
	w := term.NewWriter(&recorder{})
	defer func() { _ = w.Close() }()
	if got := w.Queued(); got != 0 {
		t.Fatalf("= %d before anything was queued", got)
	}
	for i := 1; i <= 3; i++ {
		if got := w.Queue([]byte("x")); got != uint64(i) {
			t.Fatalf("frame %d was given sequence %d", i, got)
		}
		if got := w.Queued(); got != uint64(i) {
			t.Fatalf("after %d frames the count is %d", i, got)
		}
	}
}
