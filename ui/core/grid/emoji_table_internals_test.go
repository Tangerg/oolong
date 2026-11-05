package grid

import (
	"os"
	"strconv"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"
)

func TestEmojiPresentationTableMatchesUnicodeData(t *testing.T) {
	data, err := os.ReadFile("testdata/emoji-variation-sequences.txt")
	if err != nil {
		t.Fatal(err)
	}
	bases := make(map[rune]bool)
	for line := range strings.SplitSeq(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 5 || fields[1] != "FE0F" || fields[3] != "emoji" || fields[4] != "style;" {
			continue
		}
		value, parseErr := strconv.ParseInt(fields[0], 16, 32)
		if parseErr != nil || value < 0 || value > utf8.MaxRune {
			t.Fatalf("invalid Unicode base %q", fields[0])
		}
		base := rune(value)
		bases[base] = true
		if width := ClusterWidth(string(base) + string(emojiPresentationSelector)); width != 2 {
			t.Errorf("Unicode emoji presentation %U has width %d, want 2", base, width)
		}
	}
	if len(bases) == 0 {
		t.Fatal("Unicode data supplied no emoji presentation sequences")
	}
	for base := rune(0); base <= utf8.MaxRune; base++ {
		if got := unicode.Is(emojiPresentationBases, base); got != bases[base] {
			t.Fatalf("generated presentation membership for %U = %v, source data = %v", base, got, bases[base])
		}
	}
}
