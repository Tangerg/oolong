package ansi_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/Tangerg/oolong/ui/core/ansi"
)

func TestOnlyOSCEndsAtBell(t *testing.T) {
	for _, family := range []byte{']', 'P', '_', '^', 'X'} {
		for _, size := range []int{8, 1 << 17} {
			t.Run(fmt.Sprintf("%c/%d", family, size), func(t *testing.T) {
				prefix := "\x1b" + string(family) + strings.Repeat("x", size)
				suffix := "\aHIDDEN\x1b\\visible"
				want := "visible"
				if family == ']' {
					want = "HIDDENvisible"
				}
				if got := plainPieces(t, prefix+suffix); got != want {
					t.Fatalf("Next exposed %q, want %q", got, want)
				}
				for _, chunks := range [][]string{{prefix + suffix}, {prefix, "\aHIDDEN\x1b", "\\visible"}} {
					got, err := scannedText(chunks)
					if got != want || (size > 1<<16) != errors.Is(err, ansi.ErrSequenceTooLong) {
						t.Fatalf("Scanner exposed %q with %v, want %q", got, err, want)
					}
				}
			})
		}
	}
}

func TestDCSCancellationRestoresTextWithoutCompletingAReply(t *testing.T) {
	for _, cancel := range []byte{0x18, 0x1a} {
		for _, size := range []int{8, 1 << 17} {
			t.Run(fmt.Sprintf("%x/%d", cancel, size), func(t *testing.T) {
				prefix := "\x1bP>|" + strings.Repeat("x", size)
				suffix := string(cancel) + "visible"
				if got := plainPieces(t, prefix+suffix); got != "visible" {
					t.Fatalf("Next after cancellation = %q", got)
				}
				for _, chunks := range [][]string{{prefix + suffix}, {prefix, string(cancel), "visible"}} {
					got, err := scannedText(chunks)
					if got != "visible" || (size > 1<<16) != errors.Is(err, ansi.ErrSequenceTooLong) {
						t.Fatalf("Scanner after cancellation = %q, %v", got, err)
					}
				}
			})
		}
	}
}

func plainPieces(t *testing.T, source string) string {
	t.Helper()
	var plain strings.Builder
	for source != "" {
		piece, size, complete := ansi.Next(source)
		if !complete || size == 0 {
			t.Fatalf("complete input left pending: %q", source)
		}
		if piece.Kind == ansi.Plain {
			plain.WriteString(piece.Raw)
		}
		source = source[size:]
	}
	return plain.String()
}

func scannedText(chunks []string) (string, error) {
	var scanner ansi.Scanner
	var plain strings.Builder
	var failure error
	for _, chunk := range chunks {
		err := scanner.Feed(chunk, func(piece ansi.Piece) error {
			if piece.Kind == ansi.Plain {
				plain.WriteString(piece.Raw)
			}
			return nil
		})
		failure = errors.Join(failure, err)
	}
	return plain.String(), failure
}
