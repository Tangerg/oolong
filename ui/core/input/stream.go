package input

import (
	"time"

	"github.com/Tangerg/oolong/ui/core/clipboard"
)

// Stream owns parser ambiguity, event timing and clipboard reply settlement together
// so transports cannot decode bytes independently of their deadlines. Drivers supply
// bytes, wait until DueAt and call Expire; Stream owns no goroutine or I/O. The zero
// value uses a fresh parser, [DefaultEscapeTimeout] and no clipboard. A Stream belongs
// to one decoder goroutine and must not be copied after first use.
type Stream struct {
	noCopy noCopy

	parser    *Parser
	grace     time.Duration
	clipboard *clipboard.Channel

	// deadline is when a waiting ambiguous sequence becomes the Escape key. waiting
	// says whether there is one, so a zero time is never mistaken for one that is due.
	deadline time.Time
	waiting  bool
}

type StreamConfig struct {
	Keyboard KeyboardDialect
	// Grace is how long an ambiguous sequence waits for the bytes that would settle
	// it. Zero uses [DefaultEscapeTimeout].
	Grace time.Duration
	// Clipboard settles an OSC 52 answer into a [Paste] when it belongs to a request
	// this session made. Nil leaves such answers as the [OSC] events they arrived as,
	// which is what a transport with no clipboard of its own wants.
	Clipboard *clipboard.Channel
}

func NewStream(cfg StreamConfig) *Stream {
	return &Stream{grace: cfg.Grace, clipboard: cfg.Clipboard, parser: NewParser(ParserConfig{Keyboard: cfg.Keyboard})}
}

// decoder is the parser this stream reads through, made on first use so that the
// zero Stream is a working one. Keeping the default here rather than in the
// constructor leaves one answer to what an absent parser means.
func (s *Stream) decoder() *Parser {
	if s.parser == nil {
		s.parser = &Parser{}
	}
	return s.parser
}

func (s *Stream) escapeGrace() time.Duration {
	if s.grace <= 0 {
		return DefaultEscapeTimeout
	}
	return s.grace
}

// Reset discards incomplete framing and its deadline at a break in transport
// ownership. No key or paste can be inferred from bytes the next reader missed.
func (s *Stream) Reset() {
	s.parser = &Parser{keyboard: s.decoder().keyboard}
	s.disarm()
}

// Feed decodes the next bytes, stamped with when they arrived.
//
// Drivers record at when the transport read completes, before queueing the bytes.
// An arrival at or after a pending deadline first settles that ambiguity.
//
// The returned slice is the caller's. Whether the parse is now waiting on time is
// answered by [Stream.DueAt], which every feed brings up to date.
func (s *Stream) Feed(chunk []byte, at time.Time) []Event {
	events := s.Expire(at)
	if len(chunk) == 0 {
		return events
	}
	events = append(events, s.settle(s.decoder().Feed(chunk), at)...)
	s.arm(at)
	return events
}

// Expire settles what only time could settle: a lone escape becomes the Escape key.
// A driver calls it when the moment [Stream.DueAt] reported has arrived.
func (s *Stream) Expire(at time.Time) []Event {
	if !s.waiting || at.Before(s.deadline) {
		return nil
	}
	events := s.settle(s.decoder().Expire(), s.deadline)
	s.arm(at)
	return events
}

// Flush ends the stream. A buffered escape becomes the Escape key and a half-arrived
// character is dropped, because the rest is never coming.
func (s *Stream) Flush(at time.Time) []Event {
	events := s.Expire(at)
	s.disarm()
	return append(events, s.settle(s.decoder().Flush(), at)...)
}

// DueAt is when a waiting ambiguous sequence must be settled, if one is waiting.
//
// A driver that parks until something happens has to know to wake then, or a lone
// escape is never delivered at all.
func (s *Stream) DueAt() (time.Time, bool) { return s.deadline, s.waiting }

func (s *Stream) arm(at time.Time) {
	if s.decoder().Ambiguous() {
		s.deadline, s.waiting = at.Add(s.escapeGrace()), true
		return
	}
	s.disarm()
}

func (s *Stream) disarm() { s.deadline, s.waiting = time.Time{}, false }

// Only a matching outstanding clipboard request can turn an OSC 52 reply into
// a paste. Empty or unreadable answers remain commands so they cannot clear the
// user's selection. Arrival time remains the transport's read-completion time.
func (s *Stream) settle(events []Event, at time.Time) []Event {
	events = Stamp(events, at)
	if s.clipboard == nil {
		return events
	}
	for i, event := range events {
		osc, ok := event.(OSC)
		if !ok {
			continue
		}
		if pasted, ok := osc.Paste(s.clipboard, at); ok {
			events[i] = pasted
		}
	}
	return events
}
