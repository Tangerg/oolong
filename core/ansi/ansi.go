// Package ansi is what an escape sequence is made of.
//
// Escape sequences travel in both directions and carry many meanings. What their
// readers share is syntax: which bytes are parameters, which byte ends a sequence,
// what an empty field means, and where a sequence stops. This package owns only
// that syntax so every semantic decoder can build on the same byte-level rules.
//
// Nothing here decides what a sequence means. A final byte of 'm' is a style to
// one reader and nothing at all to the other, and this package has no opinion
// about either.
//
// # What a sequence looks like
//
// Three shapes, and everything else is text:
//
//	ESC [ 1 ; 31 m          a control sequence: parameters, then a final byte
//	ESC ] 8 ; ; url ST      a string command: a body, then a terminator
//	ESC ( B                 an escape with intermediates and a final byte
package ansi

import "strings"

const (
	// Escape introduces every sequence there is.
	Escape = 0x1b
	// Bell ends a string command on the terminals that never adopted the standard
	// terminator, which is most of them.
	Bell = 0x07
)

// Body reports whether b may appear between a control sequence's introducer and
// the byte that ends it: an intermediate byte, which selects a variant of the
// sequence, or a parameter byte, which is a digit, a separator, or the marker a
// private sequence opens with.
//
// The two halves are not separately exported. Nothing outside this package has
// ever needed to tell them apart — what a reader asks is "is this still the body"
// and then "is this the end" — and a predicate nobody calls is a predicate nobody
// keeps true.
func Body(b byte) bool { return intermediate(b) || parameter(b) }

func intermediate(b byte) bool { return b >= 0x20 && b <= 0x2f }

func parameter(b byte) bool { return b >= 0x30 && b <= 0x3f }

// Final reports whether b ends a control sequence and says what it was.
func Final(b byte) bool { return b >= 0x40 && b <= 0x7e }

// Kind says what a piece of a stream turned out to be.
type Kind uint8

const (
	// Plain is text carrying no sequence.
	Plain Kind = iota
	// Control is ESC [ — parameters and a final byte. Styling, cursor movement,
	// erasure and mode changes are all this shape.
	Control
	// String is a command with a body and a terminator: an operating system
	// command, a device control string, and the three like them. [Piece.Final]
	// holds the byte that introduced it, because that is what says which.
	String
	// Other is an escape with intermediates and a final byte and no parameters:
	// selecting a character set, entering keypad mode, saving the cursor.
	Other
	// Malformed is bytes that began like a sequence and cannot be one. They are a
	// piece of their own rather than text, because a reader that fell back to
	// reading them as text would print the introducer.
	Malformed
)

// Piece is one part of a stream: a run of text, or one sequence.
type Piece struct {
	Kind Kind
	// Raw is the piece's own bytes, whole. For a sequence it includes the
	// introducer and the terminator, so that a reader passing what it does not
	// understand through to a terminal can pass it on exactly as it came.
	Raw string
	// Body is the part between the introducer and the end: the parameter section
	// of a [Control] piece, or a [String] piece's body without its terminator.
	// Empty for anything else.
	Body string
	// Final is the byte that ended a [Control] or [Other] piece and says what it
	// was, and the byte that introduced a [String] piece, for the same reason.
	Final byte
}

// Next reads the piece at the front of s.
//
// It reports the piece and how many bytes it took. The false case is the one that
// matters for anything reading a stream: what is at the front of s could still
// become a longer sequence, nothing about it can be decided until more arrives,
// and no bytes were consumed. What is left over then always begins with an escape
// byte, which is what tells a caller at the end of its input that the remainder is
// half a sequence rather than text.
//
// Text runs up to the next escape byte and no further, so a caller is handed the
// largest run it can treat as one thing.
func Next(s string) (p Piece, n int, ok bool) {
	if s == "" {
		return Piece{}, 0, false
	}
	if s[0] != Escape {
		i := strings.IndexByte(s, Escape)
		if i < 0 {
			i = len(s)
		}
		return Piece{Kind: Plain, Raw: s[:i]}, i, true
	}
	if len(s) < 2 {
		return Piece{}, 0, false
	}
	switch c := s[1]; {
	case c == '[':
		return control(s)
	case introduces(c):
		return command(s)
	case c < 0x20 || c == 0x7f:
		// A control byte cannot continue a sequence, so the escape stood alone and
		// what follows it is read on its own terms.
		return Piece{Kind: Malformed, Raw: s[:1]}, 1, true
	default:
		return escape(s)
	}
}

// introduces reports whether b introduces a string command.
//
// The five are an operating system command, a device control string, an
// application program command, a privacy message and a start of string. All of
// them are a body between an introducer and a terminator, so all of them are read
// the same way and only [Piece.Final] tells them apart.
func introduces(b byte) bool {
	switch b {
	case ']', 'P', '_', '^', 'X':
		return true
	default:
		return false
	}
}

// control reads a control sequence: a parameter section, then the byte that says
// what the sequence was.
func control(s string) (Piece, int, bool) {
	i := 2
	for i < len(s) && Body(s[i]) {
		i++
	}
	if i >= len(s) {
		return Piece{}, 0, false
	}
	if !Final(s[i]) {
		// A byte that cannot appear in a control sequence at all. What came before it
		// is dropped and reading starts again at the byte that proved it malformed,
		// which is the only reading that does not swallow whatever follows.
		return Piece{Kind: Malformed, Raw: s[:i]}, i, true
	}
	return Piece{Kind: Control, Raw: s[:i+1], Body: s[2:i], Final: s[i]}, i + 1, true
}

// command reads a string command, which ends at a bell or at an escape and a
// backslash.
//
// The C1 terminator — one byte, 0x9c — is deliberately not accepted. In UTF-8 text
// that byte is the middle of a character, and a terminator that could end a command
// halfway through a word is worse than one terminal's output going unrecognised.
func command(s string) (Piece, int, bool) {
	for i := 2; i < len(s); i++ {
		switch s[i] {
		case Bell:
			return Piece{Kind: String, Raw: s[:i+1], Body: s[2:i], Final: s[1]}, i + 1, true
		case Escape:
			if i+1 >= len(s) {
				return Piece{}, 0, false
			}
			if s[i+1] == '\\' {
				return Piece{Kind: String, Raw: s[:i+2], Body: s[2:i], Final: s[1]}, i + 2, true
			}
			// An escape inside a command that is not the terminator: the command was
			// never closed, and what is here is the start of something else.
			return Piece{Kind: Malformed, Raw: s[:i]}, i, true
		}
	}
	return Piece{}, 0, false
}

// escape reads an escape with intermediates and a final byte.
func escape(s string) (Piece, int, bool) {
	i := 1
	for i < len(s) && intermediate(s[i]) {
		i++
	}
	if i >= len(s) {
		return Piece{}, 0, false
	}
	if s[i] < 0x30 || s[i] > 0x7e {
		return Piece{Kind: Malformed, Raw: s[:i]}, i, true
	}
	return Piece{Kind: Other, Raw: s[:i+1], Final: s[i]}, i + 1, true
}
