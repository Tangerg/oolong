package term

import (
	"errors"
	"io"
	"time"

	"github.com/Tangerg/oolong/core/deadline"
	"github.com/Tangerg/oolong/core/input"
)

type readResult struct {
	data []byte
	at   time.Time
	err  error
}

// pump turns raw terminal bytes into events.
//
// It is separated from the terminal it normally reads because the interesting part
// has nothing to do with a terminal: deciding when a buffered escape has waited
// long enough is a matter of timing, and timing is what a real terminal makes
// impossible to test.
type pump struct {
	// raw preserves data and the terminal result in read-completion order.
	raw <-chan readResult
	// end is the final read consumed by the startup probe, if input ended there.
	end *readResult
	// resized carries the newest measured terminal geometry. Discovery belongs to
	// the platform watcher; the pump owns only its place in the ordered event stream.
	resized <-chan input.Resize
	// stop asks the pump to return.
	stop <-chan struct{}

	// out receives the decoded events, and is closed when the pump returns so a
	// consumer ranging over it learns that input is over.
	out chan input.Event
	// stream decodes the bytes. It is handed over rather than created here because a
	// startup probe had to read the terminal before this goroutine could, and what it
	// left behind is not only half a sequence: an escape it saw is already waiting on
	// a deadline this pump has to go on honouring.
	stream *input.Stream
	// early holds events decoded before the pump started. They are delivered from
	// here rather than pushed into out directly, because nothing is reading out
	// until this goroutine runs and a burst large enough to fill it would deadlock
	// whoever pushed.
	early []input.Event
}

// run decodes until the input ends or the pump is asked to stop and reports the
// cause. Its owner records that result before closing out, so observing the channel
// close also makes the result safe to read. EOF and an explicit stop are clean
// endings.
func (p *pump) run() error {
	if !p.send(p.early) {
		return nil
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
				return nil
			}
		case resized := <-p.resized:
			if !p.send([]input.Event{resized}) {
				return nil
			}
		case <-p.stop:
			return nil
		}
	}
}

func (p *pump) take(result readResult) (done bool, err error) {
	if !p.send(p.stream.Feed(result.data, result.at)) {
		return true, nil
	}
	if result.err == nil {
		return false, nil
	}
	p.send(p.stream.Flush(result.at))
	if errors.Is(result.err, io.EOF) {
		return true, nil
	}
	return true, result.err
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
		select {
		case p.out <- ev:
		case <-p.stop:
			return false
		}
	}
	return true
}
