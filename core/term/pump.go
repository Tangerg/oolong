package term

import (
	"sync/atomic"
	"time"

	"github.com/Tangerg/oolong/core/clipboard"
	"github.com/Tangerg/oolong/core/input"
)

// escGrace is how long a lone escape byte is held before it is taken to be the
// Escape key rather than the start of a sequence whose rest has not arrived.
//
// Long enough that a sequence sent in one burst always arrives whole, short enough
// that pressing Escape does not feel late. Escape sequences travel together; the
// gap only opens when a human is typing.
const escGrace = 30 * time.Millisecond

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
	// resized fires when the terminal's size may have changed.
	resized <-chan struct{}
	// stop asks the pump to return.
	stop <-chan struct{}

	// out receives the decoded events, and is closed when the pump returns so a
	// consumer ranging over it learns that input is over.
	out chan input.Event
	// parser decodes the bytes. It is handed over rather than created here so that
	// a startup probe, which had to read the terminal before this goroutine could,
	// can pass on a sequence that straddles the handover.
	parser *input.Parser
	// early holds events decoded before the pump started. They are delivered from
	// here rather than pushed into out directly, because nothing is reading out
	// until this goroutine runs and a burst large enough to fill it would deadlock
	// whoever pushed.
	early []input.Event
	// pasting says whether a clipboard request is outstanding, so that only an
	// answer this session asked for is turned into a paste.
	//
	// It is shared with whoever called Paste, on whatever goroutine that was, which
	// is why it is atomic and why it is a pointer: the flag belongs to the terminal
	// and the reading of it belongs here.
	pasting *atomic.Bool
	// size reports the terminal's current size.
	size func() (w, h int, err error)
	// grace overrides escGrace, for tests.
	grace time.Duration
	// now overrides the clock, for tests. It stamps keystrokes and mouse reports with
	// when they arrived, which is a fact only the reader has: a double-click, a
	// trackpad's run of wheel reports and a two-chord keybinding are all questions
	// about when something came rather than about what it was.
	now func() time.Time
}

// run decodes until the input ends or the pump is asked to stop, then closes out.
func (p *pump) run() {
	defer close(p.out)

	grace := p.grace
	if grace <= 0 {
		grace = escGrace
	}
	if !p.deliver(p.early) {
		return
	}
	p.early = nil
	parser := p.parser
	if parser == nil {
		parser = &input.Parser{}
	}

	// A stopped timer with a drained channel, so arming and disarming it is a
	// matter of Reset and Stop and never of a stale tick arriving late.
	timer := time.NewTimer(0)
	if !timer.Stop() {
		<-timer.C
	}
	armed := false
	disarm := func() {
		if armed && !timer.Stop() {
			<-timer.C
		}
		armed = false
	}

	for {
		select {
		case chunk := <-p.raw:
			disarm()
			if !p.deliver(parser.Feed(chunk)) {
				return
			}
			if parser.Pending() {
				// Something is waiting on bytes that may never come. Only time can
				// settle it.
				timer.Reset(grace)
				armed = true
			}
		case <-timer.C:
			armed = false
			if !p.deliver(parser.Flush()) {
				return
			}
		case <-p.resized:
			if w, h, err := p.size(); err == nil {
				if !p.deliver([]input.Event{input.Resize{Width: w, Height: h}}) {
					return
				}
			}
		case <-p.readErr:
			// The input is over. Bytes that arrived before it ended are still the
			// user's — they and the end arrive on separate channels, and a select
			// cannot be told to prefer one, so whichever this pass happened to see
			// first says nothing about which happened first. Everything already
			// waiting is taken before anything is given up.
			if !p.drainRaw(parser) {
				return
			}
			p.deliver(parser.Flush())
			return
		case <-p.stop:
			return
		}
	}
}

// drainRaw feeds everything already waiting on raw, reporting false when the pump was
// asked to stop part-way through.
func (p *pump) drainRaw(parser *input.Parser) bool {
	for {
		select {
		case chunk := <-p.raw:
			if !p.deliver(parser.Feed(chunk)) {
				return false
			}
		default:
			return true
		}
	}
}

// deliver sends events on, reporting false when the pump was asked to stop
// part-way through. Stopping mid-batch loses the rest, which is correct: nothing
// downstream is listening any more.
func (p *pump) deliver(events []input.Event) bool {
	for _, ev := range events {
		if pasted, ok := p.pasted(ev); ok {
			ev = pasted
		}
		switch timed := ev.(type) {
		case input.Mouse:
			if timed.At.IsZero() {
				timed.At = p.clock()
				ev = timed
			}
		case input.Key:
			if timed.At.IsZero() {
				timed.At = p.clock()
				ev = timed
			}
		}
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

// pasted turns the terminal's answer about the clipboard into the paste it means,
// when this session was the one that asked.
//
// Reading a clipboard and pasting into a terminal are the same event to whatever
// receives them, and giving them separate names would mean writing the insert
// twice. The translation happens here because this is the layer that knows what an
// operating system command is; nothing above it should have to.
//
// Only an answer that was asked for is turned into one. A terminal has no reason to
// volunteer this and none is known to, but the alternative rule — any of them is a
// paste — would let text arrive in a document nobody asked to put it in, which is
// not a thing to relax about on the strength of what terminals are known to do.
func (p *pump) pasted(ev input.Event) (input.Event, bool) {
	osc, ok := ev.(input.OSC)
	if !ok || osc.Command != clipboardCommand || p.pasting == nil || !p.pasting.Load() {
		return nil, false
	}
	p.pasting.Store(false)
	_, text, ok := clipboard.Parse(osc.Params)
	if !ok {
		// A terminal that answered with nothing readable still answered. Turning
		// that into an empty paste would clear a selection the user still has, so
		// the answer is passed through as what it is.
		return nil, false
	}
	return input.Paste{Text: text}, true
}
