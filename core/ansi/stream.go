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

// Scanner turns arbitrarily chunked terminal bytes into complete [Piece]s.
//
// Feed calls its visitor once for each complete piece, in order. A Piece and its
// strings are borrowed for that call; a visitor that retains one must clone the
// strings it needs. An incomplete escape sequence or UTF-8 character stays in the
// Scanner until another Feed completes it. Pending exposes that suffix for an
// owner settling the end of a stream.
//
// A Scanner belongs to one goroutine and must not be copied after its first use.
// Its zero value is ready to use.
type Scanner struct {
	noCopy        noCopy
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

// Feed scans chunk and visits every piece that became complete.
//
// If visit returns an error, Feed stops and returns it. The remainder is discarded:
// a semantic consumer that rejected a complete piece cannot safely resume midway
// through the same chunk.
//
// [ErrSequenceTooLong] is reported differently, because it is about the stream and
// not about the consumer. The refused sequence is consumed to its end — over as many
// chunks as that takes — and nothing of it is ever visited. Scanning carries on with
// the bytes after it, so a consumer that treats the error as "one more sequence I
// cannot use" is not thereby shown a control string's body as text.
//
// A nil visit is a programmer error and panics. Scanning without it would consume the
// chunk and advance the Scanner's held remainder, so the bytes would be gone by the
// time anyone noticed nothing had been delivered.
func (s *Scanner) Feed(chunk string, visit func(Piece) error) error {
	if visit == nil {
		panic("ansi: nil Scanner visitor")
	}
	if chunk == "" {
		return nil
	}

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
	var refused error
	for at := 0; at < len(source); {
		piece, n, ok := Next(source[at:])
		if !ok {
			// Anything unfinished and this long is an escape sequence: an unfinished
			// character is three bytes at the outside.
			tail := source[at:]
			if len(tail) > maxPending {
				return errors.Join(refused, s.refuse(tail))
			}
			if buffered && at == 0 {
				return refused
			}
			s.hold(tail)
			return refused
		}
		at += n
		// A refused sequence ends here, and only here is where it ends known. What
		// follows it is ordinary and is read as such.
		if s.refusing {
			s.refusing = false
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
			s.Reset()
			return err
		}
	}
	s.Reset()
	return refused
}

// refuse gives up on a sequence longer than any sequence may be, while going on
// looking for where it ends.
//
// What is kept is what the rest of the sequence has to be read against, and nothing
// more: the introducer, which says which terminator ends it, and the point the body
// had reached in its own syntax. Keeping the body is the thing the bound exists to
// refuse; keeping only the introducer is not enough. A control sequence that had got
// as far as its intermediate bytes is why the byte after them proves it malformed,
// and a scan that forgot them reads that byte as an ordinary parameter and swallows
// what follows it — so the same stream says different things depending on where the
// read split, which is the thing this bound exists to prevent.
//
// Both facts are read off the bytes being discarded rather than off the scan's own
// state, because a sequence long enough to refuse in a single chunk was never
// scanned incrementally at all.
//
// The error is reported once for the sequence rather than once per chunk it goes on
// arriving in.
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
func (s *Scanner) Reset() {
	s.held.Reset()
	s.scanned = 0
	s.intermediates = false
	s.refusing = false
}

func (s *Scanner) hold(tail string) {
	s.Reset()
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
	if s.scanned < start {
		s.scanned = start
	}
	for s.scanned < len(source) {
		at := s.scanned
		b := source[at]
		switch {
		case source[1] == '[':
			if intermediate(b) {
				s.intermediates = true
			} else if !parameter(b) || s.intermediates {
				return false
			}
		case introduces(source[1]):
			if b == Bell {
				return false
			}
			if b == Escape {
				return at+1 == len(source)
			}
		default:
			if !intermediate(b) {
				return false
			}
		}
		s.scanned++
	}
	return true
}
