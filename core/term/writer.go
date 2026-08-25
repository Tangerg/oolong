package term

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Tangerg/oolong/core/internal/fifo"
)

// ErrClosed marks a frame that was handed over after the writer became unusable,
// or that was still queued when its shutdown grace period ran out. Such a frame is
// abandoned rather than written. When a terminal write caused the transition,
// [Writer.Err] preserves that original failure.
var ErrClosed = errors.New("term: writer closed")

// ErrDrainTimeout means accepted frames did not settle before their caller's
// deadline. The caller still owns the display: handing it elsewhere would let a
// late frame arrive in the next owner's output.
var ErrDrainTimeout = errors.New("term: writer did not drain")

// DrainGrace is how long to wait for queued frames to reach the terminal before
// abandoning them. A terminal that has stopped accepting bytes must not be able to
// hold up an exit, and no amount of waiting makes one start accepting them again.
//
// It is what [Writer.Close] waits, and the right answer for anyone else with a
// reason to wait for the terminal to catch up.
const DrainGrace = 250 * time.Millisecond

// Writer writes frames to the terminal from a goroutine of its own.
//
// The reason it exists is that a terminal write can block for a long time — a
// remote session, a suspended emulator, a scrolled-back pager — so producers must
// not perform those writes synchronously.
//
// Successful completion is reported as a watermark rather than as a stream of
// results: the only question anyone asks is how far the terminal has got, and a
// counter answers it without a queue to keep in order or a consumer to keep up.
// [Writer.Changes] signals when that watermark or [Writer.Err] may have changed.
//
// Every method is safe for concurrent use. Frames are assigned a sequence while
// they are appended to one FIFO, so sequence order and write order cannot diverge
// when several goroutines queue terminal commands at once. A Writer must not be
// copied after construction; its queue, watermarks and worker are one publication
// owner.
type Writer struct {
	dst io.Writer

	// queue is an unbounded FIFO rather than a buffered channel. Queue must never
	// stop the interface because a terminal stopped accepting bytes, and a channel
	// large enough for every possible burst does not exist. wake says the FIFO
	// changed; the contents and the closed transition live under queueMu.
	queueMu sync.Mutex
	queue   fifo.Queue[frame]
	wake    chan struct{}
	closed  bool

	// changes holds at most one pending wake-up: a consumer that has not yet noticed
	// the last advance does not need to be told twice. It has exactly one consumer,
	// and a wake-up taken from it is gone.
	changes chan struct{}

	// advanced is closed and replaced every time the watermark moves.
	//
	// It exists because [Writer.Drain] also waits for the watermark, and a second
	// consumer of changes would take wake-ups the primary consumer is owed. A closed channel
	// is a broadcast: every waiter sees it, and none can take it from another.
	// Waiting on progress from two places was a hang that only appeared on a
	// machine slow enough for Drain to reach the channel first.
	advanceMu sync.Mutex
	advanced  chan struct{}

	queued    atomicSequence
	written   atomic.Uint64
	processed atomic.Uint64
	// settleMu turns completions back into a contiguous watermark. Writes finish in
	// sequence, but Queue calls made after Close are refused by their callers and can
	// therefore finish out of order with a write already in progress.
	settleMu sync.Mutex
	settled  map[uint64]struct{}

	// discarding tells the write goroutine to fail queued frames instead of writing
	// them once the grace period has passed. A write already in dst cannot be
	// interrupted, but everything still in the FIFO can be abandoned deterministically.
	discarding atomic.Bool

	mu      sync.Mutex
	failure error

	loopDone  chan struct{}
	closeOnce sync.Once
	closeErr  error
}

// frame is one queued payload with the sequence reserved for it.
type frame struct {
	seq  uint64
	data []byte
}

// NewWriter starts a writer over dst. The caller ends it with [Writer.Close].
//
// A nil destination is a programmer error and panics here. A writer accepts frames
// from the goroutine that draws them and writes on one of its own, so a nil surfaced
// on first use would report the fault on that second goroutine, in a stack that no
// longer names whoever failed to open the terminal.
func NewWriter(dst io.Writer) *Writer {
	if dst == nil {
		panic("term: nil writer")
	}
	w := &Writer{
		dst:      dst,
		wake:     make(chan struct{}, 1),
		changes:  make(chan struct{}, 1),
		advanced: make(chan struct{}),
		loopDone: make(chan struct{}),
	}
	go w.run()
	return w
}

// Queue takes ownership of a frame and returns the sequence number reserved for
// it. The sequence is reserved before the goroutine can see the frame, so
// [Writer.Queued] already accounts for it when Queue returns.
//
// Queue does not wait for the terminal. A frame handed over after [Writer.Close] or
// a terminal failure is accounted for but not retained or written. The failure that
// made the writer unusable remains available from [Writer.Err].
//
// Queue panics after every uint64 sequence has been used. Continuing would return
// zero and violate the watermark protocol by making new work look older than work
// already settled.
func (w *Writer) Queue(data []byte) uint64 {
	w.queueMu.Lock()
	seq, ok := w.queued.next(math.MaxUint64)
	if !ok {
		w.queueMu.Unlock()
		panic("term: writer exhausted frame sequences")
	}
	if w.closed || w.discarding.Load() {
		w.queueMu.Unlock()
		w.finish(seq, ErrClosed)
		return seq
	}
	w.queue.Push(frame{seq: seq, data: data})
	w.queueMu.Unlock()
	w.signal()
	return seq
}

// signal wakes the writer without accumulating notifications for changes it has
// not observed yet.
func (w *Writer) signal() {
	select {
	case w.wake <- struct{}{}:
	default:
	}
}

// Changes is the stable, single-consumer notification channel for writer state. A
// receive means [Writer.Written] or [Writer.Err] may have changed; several changes
// coalesce into one wake-up, so the current values must always be read from the writer.
func (w *Writer) Changes() <-chan struct{} { return w.changes }

// Queued is the highest sequence handed to the writer.
func (w *Writer) Queued() uint64 { return w.queued.current() }

// Written is the highest sequence that reached the terminal.
func (w *Writer) Written() uint64 { return w.written.Load() }

// Err is the first write failure, or nil.
//
// A terminal that has failed a write does not recover, and a UI that cannot reach
// its terminal has nothing left to do, so this is a reason to exit rather than
// something to retry.
func (w *Writer) Err() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.failure
}

// Drain waits until every frame queued so far has been written or failed, or
// until the timeout passes.
//
// It is what to call before handing the terminal to another program, so that
// program does not find half a frame in front of it. A failed frame counts as
// drained: a broken terminal must not be able to wedge a shutdown.
//
// It takes nothing from [Writer.Changes]. Waiting here must not cost its consumer a
// wake-up it is owed, which is what the broadcast channel is for.
func (w *Writer) Drain(timeout time.Duration) error {
	if !w.drain(w.queued.current(), timeout) {
		return ErrDrainTimeout
	}
	return nil
}

func (w *Writer) drain(target uint64, timeout time.Duration) bool {
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	for {
		// The channel is taken before the watermark is read, so that an advance
		// landing between the two closes a channel this is about to wait on rather
		// than one it has already let go of.
		w.advanceMu.Lock()
		advanced := w.advanced
		w.advanceMu.Unlock()

		if w.processed.Load() >= target {
			return true
		}
		select {
		case <-advanced:
		case <-w.loopDone:
			return w.processed.Load() >= target
		case <-deadline.C:
			return false
		}
	}
}

// Close drains what it can, stops the goroutine and reports whether anything had
// to be abandoned. It is idempotent.
//
// A write already inside the terminal cannot be interrupted. When the grace period
// ends with one outstanding, Close returns without waiting for the goroutine: it
// finishes on its own, discarding what is left rather than writing it.
func (w *Writer) Close() error {
	w.closeOnce.Do(func() {
		// Closing the admission edge and taking its watermark are one transition.
		// Otherwise a producer can append after Drain's snapshot and have its frame
		// abandoned by a Close that still reports success.
		w.queueMu.Lock()
		w.closed = true
		target := w.queued.current()
		w.queueMu.Unlock()
		w.signal()

		drained := w.drain(target, DrainGrace)
		if drained {
			<-w.loopDone
		} else {
			w.discarding.Store(true)
			w.signal()
			w.closeErr = fmt.Errorf("term: %d frame(s) never reached the terminal: %w",
				target-min(target, w.written.Load()), ErrClosed)
		}
	})
	return w.closeErr
}

// run is the write goroutine.
func (w *Writer) run() {
	defer func() {
		// The goroutine is the only reader of dst. Releasing it before publishing
		// loopDone means a closed Writer cannot retain the terminal transport, while a
		// timed-out Close never clears a writer still inside its Write method.
		w.dst = nil
		close(w.loopDone)
	}()
	for {
		f, ok := w.next()
		if !ok {
			return
		}
		err := ErrClosed
		if !w.discarding.Load() {
			_, err = io.Copy(w.dst, bytes.NewReader(f.data))
		}
		if err == nil {
			w.written.Store(f.seq)
		}
		w.finish(f.seq, err)
	}
}

// next waits for the next frame. Once closed, it drains the FIFO before stopping.
// If the grace period expires, Close turns on discarding and the remaining frames
// are accounted for without being written.
func (w *Writer) next() (frame, bool) {
	for {
		w.queueMu.Lock()
		if f, ok := w.queue.Pop(); ok {
			w.queueMu.Unlock()
			return f, true
		}
		closed := w.closed
		w.queueMu.Unlock()
		if closed {
			return frame{}, false
		}
		<-w.wake
	}
}

// finish records a frame's outcome and wakes whoever is watching the watermark.
func (w *Writer) finish(seq uint64, err error) {
	if err != nil && !errors.Is(err, ErrClosed) {
		w.mu.Lock()
		if w.failure == nil {
			w.failure = err
		}
		w.mu.Unlock()
		// A failed terminal stays failed. Continuing to write would produce a
		// stream of the same error and, worse, a partly-written frame after it.
		w.discarding.Store(true)
	}
	// The wake-up is queued before the watermark moves, and the watermark moves
	// before the broadcast. That order is what makes "Drain has returned" imply
	// "every wake-up those frames owed has already been queued": Drain returns on
	// the watermark, so anything published after it could still be in flight when
	// the caller looks.
	select {
	case w.changes <- struct{}{}:
	default:
	}
	advanced := w.settle(seq)
	if !advanced {
		return
	}
	// And the broadcast, which anything else waiting on the watermark observes
	// without taking the wake-up above.
	w.advanceMu.Lock()
	settled := w.advanced
	w.advanced = make(chan struct{})
	w.advanceMu.Unlock()
	close(settled)
}

// settle records one completion and advances processed only across an unbroken
// prefix. Treating processed as the largest sequence seen would let a refused frame
// overtake an older blocked write and make Drain return before that write settled.
func (w *Writer) settle(seq uint64) bool {
	w.settleMu.Lock()
	defer w.settleMu.Unlock()

	if w.settled == nil {
		w.settled = make(map[uint64]struct{})
	}
	w.settled[seq] = struct{}{}
	previous := w.processed.Load()
	processed := previous
	for {
		next := processed + 1
		if _, ok := w.settled[next]; !ok {
			break
		}
		delete(w.settled, next)
		processed = next
	}
	if processed != previous {
		w.processed.Store(processed)
		return true
	}
	return false
}
