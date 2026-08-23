package ansi

import (
	"errors"
	"strings"
)

// ErrSequenceTooLong means an unfinished escape sequence crossed the amount a
// stream scanner will retain. A sequence that has not ended within that bound is
// more likely an accidental or hostile retention leak than terminal syntax.
var ErrSequenceTooLong = errors.New("ansi: unfinished sequence exceeds 65536 bytes")

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
	noCopy noCopy
	held   strings.Builder
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
	}
	for at := 0; at < len(source); {
		piece, n, ok := Next(source[at:])
		if !ok {
			tail := source[at:]
			if len(tail) > maxPending {
				s.held.Reset()
				return ErrSequenceTooLong
			}
			if buffered && at == 0 {
				return nil
			}
			s.hold(tail)
			return nil
		}
		at += n
		if err := visit(piece); err != nil {
			s.held.Reset()
			return err
		}
	}
	s.held.Reset()
	return nil
}

// Pending is the undecided suffix waiting for another chunk. The returned string
// is valid until the next call to Feed or Reset.
func (s *Scanner) Pending() string { return s.held.String() }

// Reset drops an undecided suffix and returns the Scanner to its zero state.
func (s *Scanner) Reset() { s.held.Reset() }

func (s *Scanner) hold(tail string) {
	s.held.Reset()
	s.held.Grow(len(tail))
	s.held.WriteString(tail)
}
