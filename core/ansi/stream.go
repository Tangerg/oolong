package ansi

import (
	"errors"
	"strings"
	"unicode/utf8"
)

// ErrSequenceTooLong means an escape sequence crossed the amount a stream scanner
// will carry. A sequence that long is more likely an accidental or hostile
// retention leak than terminal syntax.
//
// The bound is on the sequence and not on what happens to be held, because a
// scanner's answer may not depend on where a read split. A sequence that arrived
// whole and one assembled from sixteen chunks are the same bytes, and the one that
// was refused while incomplete cannot become acceptable by finishing.
var ErrSequenceTooLong = errors.New("ansi: sequence exceeds 65536 bytes")

const maxPending = 1 << 16

// Scanner assembles [Piece]s across arbitrary input chunks. Pieces are borrowed
// only during the visitor call; retained strings must be cloned. Incomplete escapes
// and UTF-8 stay pending. The zero value is ready. A Scanner belongs to one goroutine
// and must not be copied after first use. Visitors may read Pending; reentering
// Feed or Reset panics.
type Scanner struct {
	noCopy        noCopy
	active        bool
	held          strings.Builder
	scanned       int
	intermediates bool
	// refusing says the sequence being held has been refused for its length and
	// will not be delivered. Its introducer is still held, because a refusal has to
	// go on looking for where the sequence ends: a scanner that simply forgot would
	// be back in ordinary text at whatever byte the next read began with, and the
	// rest of a control string would come out as the text it is not.
	refusing bool
}

// noCopy makes the scanner's single-owner contract visible to go vet. Its methods
// are never called.
type noCopy struct{}

func (*noCopy) Lock()   {}
func (*noCopy) Unlock() {}

// Feed visits complete pieces in order. Visitor errors stop scanning; panics
// propagate. Both discard the remainder because a rejected chunk cannot resume
// safely. [ErrSequenceTooLong] instead consumes the entire refused sequence and
// continues, preventing its body from becoming text. A later visitor failure is
// joined with any earlier framing error. A nil visitor panics before consuming bytes.
func (s *Scanner) Feed(chunk string, visit func(Piece) error) error {
	s.requireIdle()
	if visit == nil {
		panic("ansi: nil Scanner visitor")
	}
	if chunk == "" {
		return nil
	}
	s.active = true
	defer func() { s.active = false }()

	source := chunk
	buffered := s.held.Len() > 0
	if buffered {
		s.held.WriteString(chunk)
		source = s.held.String()
		if s.incomplete(source) {
			if len(source) > maxPending {
				return s.refuse(source)
			}
			return nil
		}
	}
	return s.scan(source, visit)
}

// scan takes ownership of the buffered input before invoking a semantic consumer.
// Only an undecided suffix is returned to the scanner; an interrupted delivery cannot
// replay bytes whose effects the visitor may already have applied.
func (s *Scanner) scan(source string, visit func(Piece) error) error {
	refusing := s.refusing
	s.reset()
	var refused error
	for at := 0; at < len(source); {
		piece, n, ok := Next(source[at:])
		if !ok {
			if err := s.unfinished(source[at:]); err != nil {
				return errors.Join(refused, err)
			}
			return refused
		}
		at += n
		// A refused sequence ends here, and only here is where it ends known. What
		// follows it is ordinary and is read as such.
		if refusing {
			refusing = false
			continue
		}
		// Plain text is exempt from the bound: it is a run rather than a sequence, and
		// nothing is waiting on a terminator for it. Everything else is bounded whether
		// or not it ended, so that a chunk boundary cannot decide the answer.
		if piece.Kind != Plain && n > maxPending {
			refused = ErrSequenceTooLong
			continue
		}
		if err := visit(piece); err != nil {
			return errors.Join(refused, err)
		}
	}
	return refused
}

func (s *Scanner) unfinished(tail string) error {
	// Anything unfinished and this long is an escape sequence: an unfinished character
	// is three bytes at the outside.
	if len(tail) > maxPending {
		return s.refuse(tail)
	}
	s.hold(tail)
	return nil
}

// Retain the introducer and parameter/intermediate phase while discarding an
// oversized sequence. Forgetting either changes its terminator across chunk
// boundaries. Derive them from discarded bytes so single-chunk refusals follow
// the same rule. Report the error once per sequence.
func (s *Scanner) refuse(sequence string) error {
	kept := sequence[:min(len(sequence), 2)]
	last := sequence[len(sequence)-1]
	switch {
	case len(sequence) == len(kept):
	case last == Escape:
		// A string sequence ends at ST, which is two bytes. Dropping the first of
		// them would make the scan run on to the next terminator it found.
		kept += string(rune(Escape))
	case sequence[1] == '[' && intermediate(last):
		kept += string(rune(last))
	}
	s.held.Reset()
	s.held.WriteString(kept)
	s.scanned = min(len(kept), 2)
	if s.refusing {
		return nil
	}
	s.refusing = true
	return ErrSequenceTooLong
}

// Pending is the undecided suffix waiting for another chunk. The returned string
// is valid until the next call to Feed or Reset.
//
// A sequence already refused for its length is not part of it: an owner settling the
// end of a stream is asking what it still owes its consumer, and a refused sequence
// is owed to nobody.
func (s *Scanner) Pending() string {
	if s.refusing {
		return ""
	}
	return s.held.String()
}

// Reset drops an undecided suffix and returns the Scanner to its zero state.
// It panics when called from this Scanner's active visitor.
func (s *Scanner) Reset() {
	s.requireIdle()
	s.reset()
}

func (s *Scanner) requireIdle() {
	if s.active {
		panic("ansi: scanner mutation during Feed")
	}
}

func (s *Scanner) reset() {
	s.held.Reset()
	s.scanned = 0
	s.intermediates = false
	s.refusing = false
}

func (s *Scanner) hold(tail string) {
	s.reset()
	s.held.Grow(len(tail))
	s.held.WriteString(tail)
}

// incomplete scans only the newly arrived suffix. Next constructs the piece once
// its terminator is known, so one-byte feeds do not repeatedly scan the prefix.
func (s *Scanner) incomplete(source string) bool {
	if source[0] != Escape {
		return !utf8.FullRuneInString(source)
	}
	if len(source) < 2 {
		return true
	}
	start := 1
	if source[1] == '[' || introduces(source[1]) {
		start = 2
	}
	s.scanned = max(s.scanned, start)
	for s.scanned < len(source) {
		switch s.step(source, s.scanned) {
		case scanEnded:
			return false
		case scanWaiting:
			return true
		case scanOn:
			s.scanned++
		}
	}
	return true
}

// scanStep is what one byte says about a sequence that is still being read.
type scanStep int

const (
	scanOn scanStep = iota
	scanEnded
	scanWaiting // it cannot end in what has arrived
)

func (s *Scanner) step(source string, at int) scanStep {
	switch b := source[at]; {
	case source[1] == '[':
		return s.controlStep(b)
	case introduces(source[1]):
		return stringStep(b, at+1 == len(source))
	case intermediate(b):
		return scanOn
	default:
		return scanEnded
	}
}

// controlStep reads one byte of a control sequence's parameter section. An
// intermediate byte ends that section, so a parameter byte after one is the final byte
// rather than another parameter.
func (s *Scanner) controlStep(b byte) scanStep {
	switch {
	case intermediate(b):
		s.intermediates = true
		return scanOn
	case parameter(b) && !s.intermediates:
		return scanOn
	default:
		return scanEnded
	}
}

// stringStep reads one byte of a control string's body. An escape ends it only
// together with the byte after it, so one at the end of what has arrived is a wait.
func stringStep(b byte, last bool) scanStep {
	switch {
	case b == Bell:
		return scanEnded
	case b == Escape && last:
		return scanWaiting
	case b == Escape:
		return scanEnded
	default:
		return scanOn
	}
}
