package term

import (
	"errors"
	"io"
	"time"

	"github.com/Tangerg/oolong/core/deadline"
	"github.com/Tangerg/oolong/core/input"
)

// pump turns raw terminal bytes into events.
//
// It is separated from the terminal it normally reads because the interesting part
// has nothing to do with a terminal: deciding when a buffered escape has waited
// long enough is a matter of timing, and timing is what a real terminal makes
// impossible to test.
type pump struct {
	// raw carries byte chunks exactly as they were read.
	raw <-chan []byte
	// readErr carries the end of the input, by error or by end of file.
	readErr <-chan error
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
	// now overrides the clock, for tests. It stamps keystrokes and mouse reports with
	// when they arrived, which is a fact only the reader has: a double-click, a
	// trackpad's run of wheel reports and a two-chord keybinding are all questions
	// about when something came rather than about what it was.
	now func() time.Time
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

	due := deadline.NewTimer()
	defer due.Stop()
	for {
		due.Schedule(p.stream.DueAt())
		select {
		case chunk := <-p.raw:
			if !p.send(p.stream.Feed(chunk, p.clock())) {
				return nil
			}
		case <-due.Channel():
			if !p.send(p.stream.Expire(p.clock())) {
				return nil
			}
		case resized := <-p.resized:
			if !p.send([]input.Event{resized}) {
				return nil
			}
		case err := <-p.readErr:
			// The input is over. Bytes that arrived before it ended are still the
			// user's — they and the end arrive on separate channels, and a select
			// cannot be told to prefer one, so whichever this pass happened to see
			// first says nothing about which happened first. Everything already
			// waiting is taken before anything is given up.
			if !p.drainRaw() {
				return nil
			}
			p.send(p.stream.Flush(p.clock()))
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		case <-p.stop:
			return nil
		}
	}
}

// drainRaw feeds everything already waiting on raw, reporting false when the pump was
// asked to stop part-way through.
func (p *pump) drainRaw() bool {
	for {
		select {
		case chunk := <-p.raw:
			if !p.send(p.stream.Feed(chunk, p.clock())) {
				return false
			}
		default:
			return true
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

// clock is when it is now, as this pump reckons it.
func (p *pump) clock() time.Time {
	if p.now != nil {
		return p.now()
	}
	return time.Now()
}
