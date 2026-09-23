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
// through the same chunk. [ErrSequenceTooLong] likewise clears the runaway suffix,
// leaving the Scanner ready for a later independent chunk.
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
				s.Reset()
				return ErrSequenceTooLong
			}
			return nil
		}
	}
	for at := 0; at < len(source); {
		piece, n, ok := Next(source[at:])
		if !ok {
			tail := source[at:]
			if len(tail) > maxPending {
				s.Reset()
				return ErrSequenceTooLong
			}
			if buffered && at == 0 {
				return nil
			}
			s.hold(tail)
			return nil
		}
		// Plain text is exempt: it is a run rather than a sequence, and nothing is
		// waiting on a terminator for it. Everything else is bounded whether or not
		// it ended, so that a chunk boundary cannot decide the answer.
		if piece.Kind != Plain && n > maxPending {
			s.Reset()
			return ErrSequenceTooLong
		}
		at += n
		if err := visit(piece); err != nil {
			s.Reset()
			return err
		}
	}
	s.Reset()
	return nil
}

// Pending is the undecided suffix waiting for another chunk. The returned string
// is valid until the next call to Feed or Reset.
func (s *Scanner) Pending() string { return s.held.String() }

// Reset drops an undecided suffix and returns the Scanner to its zero state.
func (s *Scanner) Reset() {
	s.held.Reset()
	s.scanned = 0
	s.intermediates = false
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
