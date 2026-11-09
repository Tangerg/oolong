// Package ansi owns escape-sequence syntax shared by terminal input and output
// decoders. Semantic interpretation remains with each consumer.
package ansi

import (
	"strings"
	"unicode/utf8"
)

const (
	Escape = 0x1b
	// Bell remains necessary for OSC peers that do not send ST.
	Bell = 0x07
)

// Body accepts both parameter and intermediate bytes so scanners can share the
// framing rule; Piece keeps those sections separate for semantic decoding.
func Body(b byte) bool { return intermediate(b) || parameter(b) }

func intermediate(b byte) bool { return b >= 0x20 && b <= 0x2f }

func parameter(b byte) bool { return b >= 0x30 && b <= 0x3f }

func Final(b byte) bool { return b >= 0x40 && b <= 0x7e }

type Kind uint8

const (
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

type Piece struct {
	Kind Kind
	// Raw is the piece's own bytes, whole. For a sequence it includes the
	// introducer and the terminator, so that a reader passing what it does not
	// understand through to a terminal can pass it on exactly as it came.
	Raw string
	// Parameters is the parameter-byte section of a [Control] piece. Keeping it
	// separate from Intermediates prevents a semantic decoder from trying to parse
	// an intermediate byte as a number.
	Parameters string
	// Intermediates selects a variant of a [Control] piece. DECSCUSR, for example,
	// is CSI Ps SP q and therefore carries one space here.
	Intermediates string
	// Body is a [String] piece's payload without its introducer or terminator. It is
	// empty for the other piece kinds.
	Body string
	// Final is the byte that ended a [Control] or [Other] piece and says what it
	// was, and the byte that introduced a [String] piece, for the same reason.
	Final byte
}

// Next consumes nothing while the prefix could still grow into a sequence or
// UTF-8 character. Scanner retains that undecided suffix across arbitrary chunks.
func Next(s string) (p Piece, n int, ok bool) {
	if s == "" {
		return Piece{}, 0, false
	}
	if s[0] != Escape {
		i := strings.IndexByte(s, Escape)
		if i < 0 {
			i = len(s)
			if complete := completeText(s); complete < i {
				i = complete
			}
		}
		if i == 0 {
			return Piece{}, 0, false
		}
		return Piece{Kind: Plain, Raw: s[:i]}, i, true
	}
	if len(s) < 2 {
		return Piece{}, 0, false
	}
	switch c := s[1]; {
	case c == '[':
		return control(s)
	case StringFamily(c).Valid():
		return command(s)
	case c < 0x20 || c == 0x7f:
		// A control byte cannot continue a sequence, so the escape stood alone and
		// what follows it is read on its own terms.
		return Piece{Kind: Malformed, Raw: s[:1]}, 1, true
	default:
		return escape(s)
	}
}

// completeText returns the end of the complete UTF-8 prefix of s. Invalid bytes
// are complete input and remain a semantic decoder's decision; only a valid rune
// prefix missing its final bytes must wait for the next chunk.
func completeText(s string) int {
	start := len(s) - 1
	for start > 0 && !utf8.RuneStart(s[start]) {
		start--
	}
	if utf8.FullRuneInString(s[start:]) {
		return len(s)
	}
	return start
}

func control(s string) (Piece, int, bool) {
	i := 2
	for i < len(s) && parameter(s[i]) {
		i++
	}
	paramsEnd := i
	for i < len(s) && intermediate(s[i]) {
		i++
	}
	if i >= len(s) {
		return Piece{}, 0, false
	}
	if !Final(s[i]) {
		// Leave the byte that proved the sequence malformed for the next read,
		// otherwise recovery would swallow adjacent input.
		return Piece{Kind: Malformed, Raw: s[:i]}, i, true
	}
	return Piece{
		Kind:          Control,
		Raw:           s[:i+1],
		Parameters:    s[2:paramsEnd],
		Intermediates: s[paramsEnd:i],
		Final:         s[i],
	}, i + 1, true
}

func command(s string) (Piece, int, bool) {
	for i := 2; i < len(s); i++ {
		n, result := StringFamily(s[1]).Step(s[i], s[min(i+1, len(s)-1)], i+1 < len(s))
		switch result {
		case StringTerminated:
			return Piece{Kind: String, Raw: s[:i+n], Body: s[2:i], Final: s[1]}, i + n, true
		case StringCanceled:
			return Piece{Kind: Malformed, Raw: s[:i+n]}, i + n, true
		case StringIncomplete:
			return Piece{}, 0, false
		case StringContent:
		}
	}
	return Piece{}, 0, false
}

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
