package ansi_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/Tangerg/oolong/core/ansi"
)

func TestScannerPreservesTextAndSequenceBoundariesAcrossChunks(t *testing.T) {
	const input = "中 \x1b[31mred\x1b[0m"
	for size := 1; size <= len(input); size++ {
		var scanner ansi.Scanner
		var rebuilt strings.Builder
		var sequences []string
		for at := 0; at < len(input); at += size {
			err := scanner.Feed(input[at:min(at+size, len(input))], func(piece ansi.Piece) error {
				rebuilt.WriteString(piece.Raw)
				if piece.Kind != ansi.Plain {
					sequences = append(sequences, piece.Raw)
				}
				return nil
			})
			if err != nil {
				t.Fatalf("chunks of %d: %v", size, err)
			}
		}
		if pending := scanner.Pending(); pending != "" {
			t.Fatalf("chunks of %d left %q", size, pending)
		}
		if got := rebuilt.String(); got != input {
			t.Fatalf("chunks of %d rebuilt %q", size, got)
		}
		if len(sequences) != 2 || sequences[0] != "\x1b[31m" || sequences[1] != "\x1b[0m" {
			t.Fatalf("chunks of %d found sequences %q", size, sequences)
		}
	}
}

func TestScannerBoundsAndReleasesAnUnfinishedSequence(t *testing.T) {
	var scanner ansi.Scanner
	if err := scanner.Feed("\x1b]"+strings.Repeat("x", 1<<17), func(ansi.Piece) error { return nil }); !errors.Is(err, ansi.ErrSequenceTooLong) {
		t.Fatalf("runaway sequence error = %v", err)
	}
	if pending := scanner.Pending(); pending != "" {
		t.Fatalf("runaway sequence left %d bytes pending", len(pending))
	}
	var got string
	if err := scanner.Feed("after", func(piece ansi.Piece) error {
		got += piece.Raw
		return nil
	}); err != nil || got != "after" {
		t.Fatalf("scanner after runaway = %q, %v", got, err)
	}
}

func TestScannerAppendAllocationsDoNotFollowChunkCount(t *testing.T) {
	allocations := func(chunks int) float64 {
		return testing.AllocsPerRun(3, func() {
			var scanner ansi.Scanner
			if err := scanner.Feed("\x1b]", func(ansi.Piece) error { return nil }); err != nil {
				panic(err)
			}
			for range chunks {
				if err := scanner.Feed("x", func(ansi.Piece) error { return nil }); err != nil {
					panic(err)
				}
			}
		})
	}
	small := allocations(1 << 10)
	large := allocations(2 << 10)
	if large > small+4 {
		t.Fatalf("doubling chunks grew allocations from %.0f to %.0f", small, large)
	}
}
