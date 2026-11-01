package uitest

import (
	"strings"
	"testing"
)

func TestVisibleRunsJoinAppearanceBoundaries(t *testing.T) {
	frame := "\x1b[1;1H\x1b[31mhello \x1b]8;;https://example.test\x1b\\world\x1b]8;;\x1b\\\x1b[0m"
	got, err := visibleRuns(frame)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "hello world") {
		t.Fatalf("repaint text = %q, want text joined across appearance", got)
	}
}

func TestVisibleRunsKeepGeometryBoundaries(t *testing.T) {
	for _, seq := range []string{"\x1b[2;1H", "\x1b[?6h", "\x1b[8;10;20t", "\x1b]1337;File=inline=1:AAAA\x07", "\x1bPgraphics\x1b\\"} {
		got, err := visibleRuns("hello " + seq + "world")
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(got, "hello world") {
			t.Errorf("%q joined text across geometry operation %q", got, seq)
		}
	}
}

func TestTextAssertionCannotInterpretCustomCharacterSetsAsASCII(t *testing.T) {
	if got, err := visibleRuns("\x1b(0qqq"); err == nil {
		t.Fatalf("reported %q for line-drawing glyphs", got)
	}
}
