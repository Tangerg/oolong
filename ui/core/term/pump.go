package term

import (
	"errors"
	"io"
	"time"

	"github.com/Tangerg/oolong/ui/core/deadline"
	"github.com/Tangerg/oolong/ui/core/input"
)

type readResult struct {
	data    []byte
	at      time.Time
	firstAt *time.Time
	err     error
	restart bool
}

// A decoded console rune can span two record reads. Its encoded first byte
// carries the earlier observation; its last byte keeps the completing read's
// time. Later characters in the same result retain their own arrival.
func (r readResult) decode(stream *input.Stream) []input.Event {
	if r.firstAt == nil {
		return stream.Feed(r.data, r.at)
	}
	events := stream.Feed(r.data[:1], *r.firstAt)
	return append(events, stream.Feed(r.data[1:], r.at)...)
}

// pump receives transport results separately so Escape timing can be tested
// without a real terminal or a competing input reader.
type pump struct {
	// raw preserves data and the terminal result in read-completion order.
	raw <-chan readResult
	// end is the final read consumed by the startup probe, if input ended there.
	end *readResult
	// resized carries the newest measured terminal geometry. Discovery belongs to
	// the platform watcher; the pump owns only its place in the ordered event stream.
	resized <-chan input.Resize
	stop    <-chan struct{}

	// out receives the decoded events, and is closed when the pump returns so a
	// consumer ranging over it learns that input is over.
	out chan input.Event
	// Protocol replies outlive application delivery during terminal cleanup.
	deliveryStopped <-chan struct{}
	// stream decodes the bytes. It is handed over rather than created here because a
	// startup probe had to read the terminal before this goroutine could, and what it
	// left behind is not only half a sequence: an escape it saw is already waiting on
	// a deadline this pump has to go on honouring.
	stream *input.Stream
	// early holds events decoded before the pump started. They are delivered from
	// here rather than pushed into out directly, because nothing is reading out
	// until this goroutine runs and a burst large enough to fill it would deadlock
	// whoever pushed.
	early   []input.Event
	consume func(input.Event) (bool, error)
	failure error
}

// run decodes until the input ends or the pump is asked to stop and reports the
// cause. Its owner records that result before closing out, so observing the channel
// close also makes the result safe to read. EOF and an explicit stop are clean
// endings.
func (p *pump) run() error {
	if !p.send(p.early) {
		return p.failure
	}
	p.early = nil
	if p.end != nil {
		_, err := p.take(*p.end)
		return err
	}

	due := deadline.NewTimer()
	defer due.Stop()
	for {
		due.Schedule(p.stream.DueAt())
		select {
		case result := <-p.raw:
			if done, err := p.take(result); done {
				return err
			}
		case <-due.Channel():
			if done, err := p.drainRaw(); done {
				return err
			}
			if !p.send(p.stream.Expire(time.Now())) {
				return p.failure
			}
		case resized := <-p.resized:
			if !p.send([]input.Event{resized}) {
				return p.failure
			}
		case <-p.stop:
			return nil
		}
	}
}

func (p *pump) take(result readResult) (done bool, err error) {
	if result.restart {
		p.stream.Reset()
		return false, nil
	}
	if !p.send(result.decode(p.stream)) {
		return true, p.failure
	}
	if result.err == nil {
		return false, nil
	}
	p.send(p.stream.Flush(result.at))
	if result.err == io.EOF { //nolint:errorlint // Only bare EOF ends a reader successfully; wrapped or joined failures must survive.
		return true, p.failure
	}
	return true, errors.Join(result.err, p.failure)
}

// Queued read results precede a timer's observation of the current time.
func (p *pump) drainRaw() (done bool, err error) {
	for {
		select {
		case result := <-p.raw:
			if done, err := p.take(result); done {
				return true, err
			}
		default:
			return false, nil
		}
	}
}

// send passes decoded events on, reporting false when the pump was asked to stop
// part-way through. Stopping mid-batch loses the rest, which is correct: nothing
// downstream is listening any more.
func (p *pump) send(events []input.Event) bool {
	for _, ev := range events {
		if p.consume != nil {
			consumed, err := p.consume(ev)
			if err != nil {
				p.failure = err
				return false
			}
			if consumed {
				continue
			}
		}
		select {
		case <-p.deliveryStopped:
			continue
		default:
		}
		select {
		case p.out <- ev:
		case <-p.deliveryStopped:
		case <-p.stop:
			return false
		}
	}
	return true
}
