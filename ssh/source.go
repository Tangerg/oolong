package ssh

import (
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	charmssh "charm.land/ssh"

	"github.com/Tangerg/oolong/core/clipboard"
	"github.com/Tangerg/oolong/core/deadline"
	"github.com/Tangerg/oolong/core/input"
)

// eventSource turns the two ordered facts an SSH terminal supplies — bytes and
// window changes — into one program input stream. Window intake has its own
// latest-value mailbox so a client resize cannot block the SSH request loop behind
// a slow frame.
type eventSource struct {
	cancelled <-chan struct{}
	in        io.Reader

	windows <-chan charmssh.Window
	resize  func(charmssh.Window) (input.Resize, bool, error)
	clip    *clipboard.Channel

	events     chan input.Event
	raw        chan []byte
	read       chan error
	resized    chan input.Resize
	resizeErr  chan error
	stop       chan struct{}
	done       chan struct{}
	resizeDone chan struct{}

	closeOnce sync.Once
	err       error
}

func newEventSource(
	cancelled <-chan struct{},
	in io.Reader,
	windows <-chan charmssh.Window,
	resize func(charmssh.Window) (input.Resize, bool, error),
	clip *clipboard.Channel,
) *eventSource {
	s := &eventSource{
		cancelled:  cancelled,
		in:         in,
		windows:    windows,
		resize:     resize,
		clip:       clip,
		events:     make(chan input.Event),
		raw:        make(chan []byte, 4),
		read:       make(chan error, 1),
		resized:    make(chan input.Resize, 1),
		resizeErr:  make(chan error, 1),
		stop:       make(chan struct{}),
		done:       make(chan struct{}),
		resizeDone: make(chan struct{}),
	}
	go s.readInput()
	go s.watchWindows()
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
// it would mean waiting for the client to send something or disconnect. It takes no
// further bytes once this returns — see [eventSource.readInput] — and ends with the
// channel. The consequence belongs in the contract of [Run]: the session's input is
// this package's for the session's lifetime, not only for the call.
func (s *eventSource) Close() {
	s.closeOnce.Do(func() { close(s.stop) })
	<-s.done
	<-s.resizeDone
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
	buffer := make([]byte, 4096)
	emptyReads := 0
	for {
		if s.ending() {
			return
		}
		n, err := s.in.Read(buffer)
		if n < 0 || n > len(buffer) {
			s.reportRead(fmt.Errorf("ssh: reader returned invalid byte count %d", n))
			return
		}
		if n > 0 {
			emptyReads = 0
			if s.ending() {
				return
			}
			chunk := append([]byte(nil), buffer[:n]...)
			select {
			case s.raw <- chunk:
			case <-s.stop:
				return
			case <-s.cancelled:
				return
			}
		} else if err == nil {
			emptyReads++
			if emptyReads >= 100 {
				err = io.ErrNoProgress
			}
		}
		if err != nil {
			s.reportRead(err)
			return
		}
	}
}

// ending reports whether anyone is still waiting for what this source produces.
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

func (s *eventSource) reportRead(err error) {
	select {
	case s.read <- err:
	case <-s.stop:
	case <-s.cancelled:
	}
}

func (s *eventSource) watchWindows() {
	defer close(s.resizeDone)
	for {
		select {
		case window, ok := <-s.windows:
			if !ok {
				return
			}
			resized, changed, err := s.resize(window)
			if err != nil {
				select {
				case s.resizeErr <- err:
				case <-s.stop:
				case <-s.cancelled:
				}
				return
			}
			if changed {
				s.postResize(resized)
			}
		case <-s.stop:
			return
		case <-s.cancelled:
			return
		}
	}
}

func (s *eventSource) postResize(resized input.Resize) {
	select {
	case s.resized <- resized:
		return
	default:
	}
	select {
	case <-s.resized:
	default:
	}
	select {
	case s.resized <- resized:
	case <-s.stop:
	case <-s.cancelled:
	}
}

// run decodes the accepted session's bytes. How an escape is settled by time and how
// a clipboard answer becomes a paste belong to [input.Stream]; what is left here is
// this transport's own — which channels carry what, and when the session is over.
func (s *eventSource) run() {
	defer close(s.done)
	defer close(s.events)
	stream := input.NewStream(input.StreamConfig{Clipboard: s.clip})

	due := deadline.NewTimer()
	defer due.Stop()

	for {
		due.Schedule(stream.DueAt())
		select {
		case chunk := <-s.raw:
			if !s.deliver(stream.Feed(chunk, time.Now())) {
				return
			}
		case <-due.Channel():
			if !s.deliver(stream.Expire(time.Now())) {
				return
			}
		case resized := <-s.resized:
			if !s.deliver([]input.Event{resized}) {
				return
			}
		case err := <-s.resizeErr:
			s.err = err
			return
		case err := <-s.read:
			// Bytes that arrived before the end are still the user's, and they arrive
			// on a different channel from the end itself, so everything already waiting
			// is taken before anything is given up.
			if !s.drainRaw(stream) {
				return
			}
			s.deliver(stream.Flush(time.Now()))
			if !errors.Is(err, io.EOF) {
				s.err = err
			}
			return
		case <-s.stop:
			return
		case <-s.cancelled:
			return
		}
	}
}

// drainRaw feeds everything already waiting on raw, reporting false when the session
// ended part-way through.
func (s *eventSource) drainRaw(stream *input.Stream) bool {
	for {
		select {
		case chunk := <-s.raw:
			if !s.deliver(stream.Feed(chunk, time.Now())) {
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
