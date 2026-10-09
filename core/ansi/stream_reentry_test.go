package ansi_test

import (
	"slices"
	"testing"

	"github.com/Tangerg/oolong/core/ansi"
)

func TestScannerRejectsMutationWhileVisiting(t *testing.T) {
	for _, mutation := range []string{"feed", "empty feed", "reset"} {
		t.Run(mutation, func(t *testing.T) {
			var scanner ansi.Scanner
			var pieces []string
			err := scanner.Feed("first\x1b[31msecond", func(piece ansi.Piece) error {
				pieces = append(pieces, piece.Raw)
				func() {
					defer func() {
						if recover() == nil {
							t.Error("visitor independently changed its active scanner")
						}
					}()
					switch mutation {
					case "feed":
						_ = scanner.Feed("\x1b[", func(ansi.Piece) error { return nil })
					case "empty feed":
						_ = scanner.Feed("", func(ansi.Piece) error { return nil })
					case "reset":
						scanner.Reset()
					}
				}()
				return nil
			})
			if err != nil || !slices.Equal(pieces, []string{"first", "\x1b[31m", "second"}) {
				t.Fatalf("outer scan = %q, %v", pieces, err)
			}
			if err := scanner.Feed("next", func(ansi.Piece) error { return nil }); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestScannerConsumesBufferedInputBeforeVisiting(t *testing.T) {
	var scanner ansi.Scanner
	if err := scanner.Feed("\x1b[", func(ansi.Piece) error { return nil }); err != nil {
		t.Fatal(err)
	}
	var pieces []string
	err := scanner.Feed("31mtext\x1b]", func(piece ansi.Piece) error {
		pieces = append(pieces, piece.Raw)
		if pending := scanner.Pending(); pending != "" {
			t.Errorf("completed input remained undecided during delivery: %q", pending)
		}
		return nil
	})
	if err != nil || !slices.Equal(pieces, []string{"\x1b[31m", "text"}) || scanner.Pending() != "\x1b]" {
		t.Fatalf("scan = %q, pending %q, %v", pieces, scanner.Pending(), err)
	}
}

func TestInterruptedScannerVisitorCannotReplayConsumedInput(t *testing.T) {
	var scanner ansi.Scanner
	if err := scanner.Feed("\x1b[", func(ansi.Piece) error { return nil }); err != nil {
		t.Fatal(err)
	}
	const failure = "consumer failed"
	func() {
		defer func() {
			if got := recover(); got != failure {
				t.Fatalf("visitor panic = %v", got)
			}
		}()
		_ = scanner.Feed("31mnot delivered\x1b]", func(ansi.Piece) error { panic(failure) })
	}()
	if pending := scanner.Pending(); pending != "" {
		t.Errorf("interrupted scan kept input for replay: %q", pending)
	}
	var pieces []string
	err := scanner.Feed("new input", func(piece ansi.Piece) error {
		pieces = append(pieces, piece.Raw)
		return nil
	})
	if err != nil || !slices.Equal(pieces, []string{"new input"}) {
		t.Fatalf("interrupted input replayed: %q, %v", pieces, err)
	}
}
