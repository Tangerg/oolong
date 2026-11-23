package input

import (
	"bytes"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Tangerg/oolong/ui/core/ansi"
)

// DefaultEscapeTimeout belongs to the protocol boundary so local and remote
// transports resolve the same ambiguous bytes consistently. Parser owns no clock.
const DefaultEscapeTimeout = 30 * time.Millisecond

const esc = ansi.Escape

// introducerLen is how many bytes open a sequence: Escape and the byte that
// selects its family.
const introducerLen = 2

const (
	// Rejected OSC replies keep only a complete correlation header within the
	// ordinary parameter budget, never the oversized payload behind it.
	maxParameterBytes = 64 << 10

	// maxPaste bounds what one paste may accumulate. A terminal that opens a
	// paste and never closes it would otherwise swallow everything typed
	// afterwards into a buffer that only grows. On reaching the bound the text so
	// far is delivered as one Paste event and accumulation begins again, so the
	// allocation stays bounded without interpreting any of the payload as input.
	// Each malformed input byte can expand to a UTF-8 replacement rune.
	maxPaste = (MaxDecodedEventBytes - eventOverhead) / len(replacement)
)

// replacement stands in for malformed bytes where untrusted input becomes text.
const replacement = string(utf8.RuneError)

// KeyboardDialect owns overlapping wire meanings; enhancement flags do not
// change a protocol family's modifier namespace.
type KeyboardDialect uint8

const (
	XTermKeyboard KeyboardDialect = iota
	KittyKeyboard
)

var ErrInvalidKeyboardDialect = errors.New("input: invalid keyboard dialect")

func (d KeyboardDialect) Validate() error {
	if d > KittyKeyboard {
		return fmt.Errorf("%w: %d", ErrInvalidKeyboardDialect, d)
	}
	return nil
}

type ParserConfig struct {
	// Legacy modified F3 overlaps an unmarked cursor-position reply. Query
	// owners must use DEC private CPR (CSI ?6n) while this dialect is active.
	Keyboard KeyboardDialect
}

// NewParser panics for an invalid dialect because modifier interpretation requires a known protocol.
func NewParser(cfg ParserConfig) *Parser {
	if err := cfg.Keyboard.Validate(); err != nil {
		panic(err)
	}
	return &Parser{keyboard: cfg.Keyboard}
}

// Once a payload introducer is complete, its bytes cannot regain keyboard
// meaning through a timeout. One frame kind owns that interpretation until the
// payload ends; paste, mouse, overrun and control-string bodies are mutually
// exclusive.
type frameKind uint8

const (
	noFrame frameKind = iota
	pasteFrame
	x10MouseFrame
	// overrunFrame is a control sequence whose parameter section overran what one
	// may hold. It ends at the first byte a sequence could have ended with.
	overrunFrame
	// stringFrame accumulates [Parser.str].
	stringFrame
)

// Parser retains ambiguous bytes across arbitrary chunks so split escape sequences
// and UTF-8 cannot lose keys. A lone escape needs an external deadline: Pending
// reports it and Flush ends the wait. A Parser belongs to one reader goroutine,
// must not be copied after first use, and is not safe for concurrent use.
type Parser struct {
	noCopy   noCopy
	keyboard KeyboardDialect

	buf       []byte
	controlAt int
	frame     frameKind
	paste     []byte
	str       controlString
	arrivals  []arrivedBytes
	origin    arrival
	hasOrigin bool
}

// noCopy makes the parser's single-owner contract visible to go vet. Its methods
// are never called.
type noCopy struct{}

func (*noCopy) Lock()   {}
func (*noCopy) Unlock() {}

func (p *Parser) Feed(b []byte) []Event {
	return eventsOnly(p.feed(b, time.Time{}))
}

func (p *Parser) feed(b []byte, at time.Time) []receivedEvent {
	p.buffer(b, at)
	return p.drain(inputAvailable)
}

// Flush resolves pending input at the end of the byte stream.
// Use Ambiguous and Expire for an Escape timer while input can still arrive.
//
// A buffered escape becomes the Escape key, and anything after it is re-read as
// ordinary input. A half-arrived character is dropped, since the rest is never
// coming. A paste in progress is left alone: it is incomplete rather than
// ambiguous, and cutting it short would corrupt the text.
// Incomplete control strings and X10 reports are discarded because their payloads
// cannot become keys, and EOF cannot authorize a protocol reply.
func (p *Parser) Flush() []Event { return eventsOnly(p.drain(inputEnded)) }

// Pending reports whether the parser is waiting for more input. It does not
// indicate a timeout: partial UTF-8 and paste data must continue waiting. Arm an
// Escape timer only when Ambiguous reports true, and call Expire when it fires.
//
// An open paste and a string being accumulated count even with nothing buffered.
// Their bytes have been consumed into the payload rather than left in the buffer,
// so a report drawn from the buffer alone would say a paste that had not reached
// its terminator was nothing at all.
func (p *Parser) Pending() bool {
	return len(p.buf) > 0 || p.frame != noFrame
}

// Ambiguous reports whether an Escape timeout can resolve pending input.
// Incomplete UTF-8 and recognized paste, mouse or string payloads must wait for more bytes.
func (p *Parser) Ambiguous() bool {
	return p.frame == noFrame && len(p.buf) > 0 && p.buf[0] == esc
}

// Expire resolves Escape ambiguity without declaring the byte stream finished.
func (p *Parser) Expire() []Event {
	return eventsOnly(p.expire())
}

func (p *Parser) expire() []receivedEvent {
	if !p.Ambiguous() {
		return nil
	}
	return p.drain(escapeExpired)
}

type parseTrigger uint8

const (
	inputAvailable parseTrigger = iota
	escapeExpired
	inputEnded
)

func (p *Parser) drain(trigger parseTrigger) (events []receivedEvent) {
	initial := len(p.buf)
	observations := len(p.arrivals)
	defer func() {
		if len(p.buf) > 0 && len(p.buf) < initial/2 {
			// The undecided tail is short by construction. Clone it after draining so
			// a lone escape or partial UTF-8 sequence cannot keep an otherwise consumed
			// terminal read buffer alive while input is idle.
			p.buf = bytes.Clone(p.buf)
		}
		if len(p.arrivals) > 0 && len(p.arrivals) < observations/2 {
			p.arrivals = slices.Clone(p.arrivals)
		}
	}()
	for {
		event, advanced := p.advance(trigger)
		if !advanced {
			return events
		}
		if event != nil {
			events = append(events, receivedEvent{event: event, arrival: p.origin})
		}
		if p.frame == noFrame {
			p.origin, p.hasOrigin = arrival{}, false
		}
	}
}

// advance makes one state transition. Its boolean distinguishes waiting from
// consuming bytes that deliberately produce no event.
func (p *Parser) advance(trigger parseTrigger) (Event, bool) {
	switch p.frame {
	case pasteFrame:
		return p.advancePaste()
	case x10MouseFrame:
		return p.advanceX10Mouse(trigger)
	case overrunFrame:
		return p.advanceOverrun(trigger)
	case stringFrame:
		return p.advanceString(trigger)
	default:
		return p.advanceInput(trigger)
	}
}

func (p *Parser) advanceOverrun(trigger parseTrigger) (Event, bool) {
	if p.skipParams() {
		return nil, true
	}
	if trigger == inputEnded {
		p.frame = noFrame
		return nil, true
	}
	return nil, false
}

func (p *Parser) advanceString(trigger parseTrigger) (Event, bool) {
	if event, advanced := p.readString(); advanced {
		return event, true
	}
	if trigger != inputEnded {
		return nil, false
	}
	p.take(len(p.buf))
	p.abandonString()
	return nil, true
}

func (p *Parser) advancePaste() (Event, bool) {
	text, done := p.readPaste()
	if !done || text == "" {
		return nil, done
	}
	// A paste is bytes a terminal was handed by something else, and Text is a
	// string, which every consumer will treat as text and eventually put in a cell.
	// Invalid UTF-8 is replaced rather than passed on, for the same reason a control
	// character is dropped at the cell: this is where untrusted bytes stop being
	// untrusted.
	return Paste{Text: strings.ToValidUTF8(text, replacement)}, true
}

func (p *Parser) advanceInput(trigger parseTrigger) (Event, bool) {
	if len(p.buf) == 0 {
		return nil, false
	}
	n, event, done := p.decode(p.buf, trigger)
	if done {
		p.take(n)
		return event, true
	}
	if trigger == inputAvailable || trigger == escapeExpired && p.buf[0] != esc {
		return nil, false
	}
	if p.buf[0] == esc {
		p.take(1)
		return Key{Code: Esc}, true
	}
	// A character whose remaining bytes will never arrive. Dropping one byte rather
	// than the buffer keeps whatever follows decodable.
	p.take(1)
	return nil, true
}

// skipParams drops the rest of a control sequence whose parameter section
// overran, and reports whether it found the end.
//
// The bytes are dropped rather than re-read as text. A stream of parameter bytes
// would otherwise arrive as a flood of keystrokes, which is a worse answer to a
// malformed sequence than silence — and one that a hostile terminal could aim.
func (p *Parser) skipParams() bool {
	i := 0
	for i < len(p.buf) && ansi.Body(p.buf[i]) {
		i++
	}
	if i >= len(p.buf) {
		p.take(i)
		return false
	}
	// The byte that ended the run of parameter bytes ends the sequence too, if it
	// is one a sequence could have ended with. Anything else proved the sequence
	// malformed and is left to be read on its own terms.
	if ansi.Final(p.buf[i]) {
		i++
	}
	p.take(i)
	p.frame = noFrame
	return true
}

// take drops n decoded bytes, releasing the buffer once it is empty so an idle
// parser holds nothing.
func (p *Parser) take(n int) {
	p.consumeArrival(n)
	p.controlAt = 0
	p.buf = p.buf[n:]
	if len(p.buf) == 0 {
		p.buf = nil
	}
}
