package programtest

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
	got, err := visibleRuns("\x1b[1;1Hhello \x1b[2;1Hworld")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, "hello world") {
		t.Fatalf("repaint text = %q, joined text across cursor movement", got)
	}
}
