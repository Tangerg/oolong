package ssh

import (
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/Tangerg/oolong/ui/core/clipboard"
	"github.com/Tangerg/oolong/ui/core/deadline"
	"github.com/Tangerg/oolong/ui/core/input"
)

type readResult struct {
	data []byte
	at   time.Time
	err  error
}

var errReadInterrupted = errors.New("ssh: input read interrupted; reader ended")

// eventSource turns the two ordered facts an SSH terminal supplies — bytes and
// window changes — into one program input stream. Window intake has its own
// latest-value mailbox so a client resize cannot block the SSH request loop behind
// a slow frame.
type eventSource struct {
	cancelled <-chan struct{}
	in        io.Reader

	windows *windowFeed
	clip    *clipboard.Channel

	events chan input.Event
	raw    chan readResult
	stop   chan struct{}
	done   chan struct{}

	closeOnce sync.Once
	err       error
}

func newEventSource(
	cancelled <-chan struct{},
	in io.Reader,
	windows *windowFeed,
	clip *clipboard.Channel,
) *eventSource {
	s := &eventSource{
		cancelled: cancelled,
		in:        in,
		windows:   windows,
		clip:      clip,
		events:    make(chan input.Event),
		raw:       make(chan readResult, 4),
		stop:      make(chan struct{}),
		done:      make(chan struct{}),
	}
	go s.readInput()
	go s.run()
	return s
}

func (s *eventSource) Events() <-chan input.Event { return s.events }

// Err reports the terminal result after Events closes. Channel closure
// synchronizes run's write with this read.
func (s *eventSource) Err() error { return s.err }

// Close stops decoding and waits for everything this source owns the end of.
//
// It does not wait for the session reader. That goroutine may be inside a read of
// an SSH channel, which has no deadline and belongs to the caller, so waiting for
// it would mean waiting for the client to send something or disconnect. It begins no
// further read once this returns — see [eventSource.readInput] — and the one already
// in flight ends with the channel. This is why [Terminal.Open] requires exclusive use of
// session input for the remaining channel lifetime.
func (s *eventSource) Close() {
	s.stopIntake()
	<-s.done
}

func (s *eventSource) stopIntake() {
	s.closeOnce.Do(func() { close(s.stop) })
}

// readInput takes the session's bytes until the session ends or nobody is left to
// want them.
//
// An SSH channel cannot be read with a deadline and this package does not own the
// channel, so one read can still be in flight when the source is closed — see
// [eventSource.Close]. That one is unavoidable; every one after it is not, which is
// why ending is checked before a chunk is offered and again before the next read.
// Offering it in a select alongside the stop signal would let a closed source go on
// taking the caller's bytes for as long as the buffer had room.
func (s *eventSource) readInput() {
	completed := false
	defer func() {
		if !completed {
			// The reader owns the ordered terminal result even when Read unwinds.
			// Earlier accepted chunks still precede this failure in the raw stream.
			err := errReadInterrupted
			if v := recover(); v != nil {
				cause, ok := v.(error)
				if !ok {
					cause = fmt.Errorf("panic: %v", v)
				}
				err = errors.Join(err, cause)
			}
			s.offer(readResult{at: time.Now(), err: err})
		}
	}()
	s.readChunks()
	completed = true
}

func (s *eventSource) readChunks() {
	buffer := make([]byte, 4096)
	emptyReads := 0
	for {
		if s.ending() {
			return
		}
		n, err := s.in.Read(buffer)
		at := time.Now()
		if n < 0 || n > len(buffer) {
			err = fmt.Errorf("ssh: reader returned invalid byte count %d", n)
			n = 0
		}
		if n > 0 {
			emptyReads = 0
		} else if err == nil {
			emptyReads++
			if emptyReads >= 100 {
				err = io.ErrNoProgress
			}
		}
		if n > 0 || err != nil {
			result := readResult{data: append([]byte(nil), buffer[:n]...), at: at, err: err}
			if !s.offer(result) {
				return
			}
		}
		if err != nil {
			return
		}
	}
}

// Check ownership before offering a completed read; a select alone could keep
// accepting bytes when both a buffered send and the stop signal are ready.
func (s *eventSource) offer(result readResult) bool {
	if s.ending() {
		return false
	}
	select {
	case s.raw <- result:
		return true
	case <-s.stop:
	case <-s.cancelled:
	}
	return false
}

func (s *eventSource) ending() bool {
	select {
	case <-s.stop:
		return true
	case <-s.cancelled:
		return true
	default:
		return false
	}
}

// run decodes the accepted session's bytes. How an escape is settled by time and how
// a clipboard answer becomes a paste belong to [input.Stream]; what is left here is
// this transport's own — which channels carry what, and when the session is over.
func (s *eventSource) run() {
	defer close(s.done)
	defer close(s.events)
	defer s.stopIntake()
	var resized <-chan input.Resize
	if s.windows != nil {
		resized = s.windows.events
	}
	stream := input.NewStream(input.StreamConfig{Clipboard: s.clip})

	due := deadline.NewTimer()
	defer due.Stop()

	for {
		due.Schedule(stream.DueAt())
		select {
		case result := <-s.raw:
			if !s.take(stream, result) {
				return
			}
		case <-due.Channel():
			if !s.drainRaw(stream) || !s.deliver(stream.Expire(time.Now())) {
				return
			}
		case size, ok := <-resized:
			if !ok {
				s.err = s.windows.result()
				return
			}
			if !s.deliver([]input.Event{size}) {
				return
			}
		case <-s.stop:
			return
		case <-s.cancelled:
			return
		}
	}
}

func (s *eventSource) take(stream *input.Stream, result readResult) bool {
	if !s.deliver(stream.Feed(result.data, result.at)) {
		return false
	}
	if result.err == nil {
		return true
	}
	s.deliver(stream.Flush(result.at))
	if result.err != io.EOF { //nolint:errorlint // Only bare EOF ends a reader successfully; wrapped or joined failures must survive.
		s.err = result.err
	}
	return false
}

func (s *eventSource) drainRaw(stream *input.Stream) bool {
	for {
		select {
		case result := <-s.raw:
			if !s.take(stream, result) {
				return false
			}
		default:
			return true
		}
	}
}

func (s *eventSource) deliver(events []input.Event) bool {
	for _, event := range events {
		select {
		case s.events <- event:
		case <-s.stop:
			return false
		case <-s.cancelled:
			return false
		}
	}
	return true
}
