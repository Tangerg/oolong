package input

import (
	"time"

	"github.com/Tangerg/oolong/core/clipboard"
)

// Stream turns a transport's byte stream into the events an interface handles.
//
// Every transport that reads a terminal — the local one, an accepted SSH session,
// anything else — has the same three decisions to make, and only the bytes differ:
// when an ambiguous escape has waited long enough to be the Escape key, when an event
// arrived, and whether a clipboard answer settles a request this session made. A
// transport that made them itself would be a second place those rules live, and the
// two would agree only for as long as somebody kept checking.
//
// It is state, not machinery: it owns no goroutine, reads nothing, and waits for
// nothing. Its driver hands it bytes, asks [Stream.DueAt] when it must be woken
// again, and calls [Stream.Expire] when that moment arrives. What remains transport-
// specific — which channels carry the bytes, how a stop is signalled, where the
// events go — stays with the transport, where it belongs.
//
// The zero value is a decoder of its own: a fresh parser, [DefaultEscapeTimeout], and
// no clipboard. [NewStream] is for a transport that has something to say about either
// of the last two. The parser is never anyone else's: a half-decoded sequence and the
// moment its ambiguity runs out are one fact, and a transport that could be handed a
// parser without its deadline would be given a decoder whose waiting nobody was doing.
//
// A Stream belongs to whichever goroutine reads the transport and must not be copied
// after first use: its parser and its deadline are one decoder.
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

// StreamConfig owns the complete construction contract of a [Stream].
type StreamConfig struct {
	// Grace is how long an ambiguous sequence waits for the bytes that would settle
	// it. Zero uses [DefaultEscapeTimeout].
	Grace time.Duration
	// Clipboard settles an OSC 52 answer into a [Paste] when it belongs to a request
	// this session made. Nil leaves such answers as the [OSC] events they arrived as,
	// which is what a transport with no clipboard of its own wants.
	Clipboard *clipboard.Channel
}

// NewStream makes a decoder for one transport. Every setting it leaves out means
// what it means in the zero Stream.
func NewStream(cfg StreamConfig) *Stream {
	return &Stream{grace: cfg.Grace, clipboard: cfg.Clipboard}
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

// escapeGrace is how long this stream waits, with the same rule.
func (s *Stream) escapeGrace() time.Duration {
	if s.grace <= 0 {
		return DefaultEscapeTimeout
	}
	return s.grace
}

// Feed decodes the next bytes, stamped with when they arrived.
//
// The returned slice is the caller's. Whether the parse is now waiting on time is
// answered by [Stream.DueAt], which every feed brings up to date.
func (s *Stream) Feed(chunk []byte, at time.Time) []Event {
	events := s.settle(s.decoder().Feed(chunk), at)
	s.arm(at)
	return events
}

// Expire settles what only time could settle: a lone escape becomes the Escape key.
// A driver calls it when the moment [Stream.DueAt] reported has arrived.
func (s *Stream) Expire(at time.Time) []Event {
	s.disarm()
	return s.settle(s.decoder().Expire(), at)
}

// Flush ends the stream. A buffered escape becomes the Escape key and a half-arrived
// character is dropped, because the rest is never coming.
func (s *Stream) Flush(at time.Time) []Event {
	s.disarm()
	return s.settle(s.decoder().Flush(), at)
}

// DueAt is when a waiting ambiguous sequence must be settled, if one is waiting.
//
// A driver that parks until something happens has to know to wake then, or a lone
// escape is never delivered at all.
func (s *Stream) DueAt() (time.Time, bool) { return s.deadline, s.waiting }

// arm re-reads the parser's ambiguity against at.
func (s *Stream) arm(at time.Time) {
	if s.decoder().Ambiguous() {
		s.deadline, s.waiting = at.Add(s.escapeGrace()), true
		return
	}
	s.disarm()
}

func (s *Stream) disarm() { s.deadline, s.waiting = time.Time{}, false }

// settle applies the two facts a transport boundary owns: when the events arrived,
// and whether a clipboard answer among them belongs to this session.
//
// Only an answer that was asked for becomes a paste. A terminal has no reason to
// volunteer one and none is known to, but the alternative rule — any of them is a
// paste — would let text arrive in a document nobody asked to put it in, which is not
// a thing to relax about on the strength of what terminals are known to do. An answer
// that arrives with nothing readable in it still answered, so it is passed through as
// the command it was rather than turned into an empty paste that would clear a
// selection the user still has.
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
		if pasted, ok := osc.Paste(s.clipboard); ok {
			events[i] = pasted
		}
	}
	return events
}
