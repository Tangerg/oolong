package ansi_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/Tangerg/oolong/core/ansi"
)

// scanChunked feeds input in fixed-size pieces and reports everything the scanner
// delivered, joined back into the bytes it came from.
//
// Where a Plain run is cut is a chunk's business — a scanner fed one byte at a time
// has one byte to deliver — so the pieces are joined rather than compared one by
// one. What must not change is the bytes and whether the stream was refused.
func scanChunked(input string, chunk int) (string, error) {
	var scanner ansi.Scanner
	var delivered strings.Builder
	visit := func(piece ansi.Piece) error {
		delivered.WriteString(piece.Raw)
		return nil
	}
	for at := 0; at < len(input); at += chunk {
		if err := scanner.Feed(input[at:min(at+chunk, len(input))], visit); err != nil {
			return delivered.String(), err
		}
	}
	return delivered.String(), nil
}

// TestAScannersAnswerDoesNotDependOnWhereTheReadSplit is the property the retention
// bound used to break.
//
// The bound was measured against what the scanner happened to be holding, so an
// escape sequence longer than it was refused when it arrived in pieces and accepted
// when it arrived whole. A terminal read splits wherever the kernel says, which made
// the same byte stream mean two different things on two different runs.
func TestAScannersAnswerDoesNotDependOnWhereTheReadSplit(t *testing.T) {
	for name, input := range map[string]string{
		"a control sequence that never ends":  "\x1b[" + strings.Repeat("1;", 40000),
		"a command that never ends":           "\x1b]" + strings.Repeat("a", 80000),
		"text before one that never ends":     "hello\x1b[" + strings.Repeat("2;", 40000),
		"a control sequence that does end":    "\x1b[" + strings.Repeat("1;", 40000) + "m",
		"a command that does end":             "\x1b]" + strings.Repeat("a", 80000) + "\x07",
		"ordinary text far past the bound":    strings.Repeat("x", 200000),
		"ordinary text with half a character": strings.Repeat("x", 200000) + "\xe4\xb8",
		"a short stream with a sequence in":   "ab\x1b[31mcd",
	} {
		whole, wholeErr := scanChunked(input, len(input))
		for _, chunk := range []int{1, 7, 1000, 60000} {
			got, err := scanChunked(input, chunk)
			switch {
			case (err == nil) != (wholeErr == nil):
				t.Errorf("%s in %d-byte chunks: %v, but whole: %v", name, chunk, err, wholeErr)
			case got != whole:
				t.Errorf("%s in %d-byte chunks delivered %d bytes, but whole delivered %d",
					name, chunk, len(got), len(whole))
			}
		}
	}
}

func TestASequenceLongerThanTheBoundIsRefusedHoweverItArrives(t *testing.T) {
	// Refusing it only while it was unfinished let a hostile stream buy itself an
	// unbounded piece by remembering to terminate it.
	input := "\x1b]" + strings.Repeat("a", 80000) + "\x07"
	for _, chunk := range []int{1, 4096, len(input)} {
		if _, err := scanChunked(input, chunk); !errors.Is(err, ansi.ErrSequenceTooLong) {
			t.Errorf("in %d-byte chunks: %v, want ErrSequenceTooLong", chunk, err)
		}
	}
}
