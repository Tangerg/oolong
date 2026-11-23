package ansi_test

import (
	"testing"

	"github.com/Tangerg/oolong/ui/core/ansi"
)

func TestCursorStyleReplyRequiresTheRequestedSuccessfulStatus(t *testing.T) {
	for _, tc := range []struct {
		body     string
		sequence string
	}{
		{"1$r0 q", "\x1b[0 q"},
		{"1$r1 q", "\x1b[1 q"},
		{"1$r2 q", "\x1b[2 q"},
		{"1$r3 q", "\x1b[3 q"},
		{"1$r4 q", "\x1b[4 q"},
		{"1$r5 q", "\x1b[5 q"},
		{"1$r6 q", "\x1b[6 q"},
		{"0$r4 q", ""},
		{"1$r4m", ""},
		{"1$r7 q", ""},
		{"1$r-1 q", ""},
		{"1$r4 q\x1b[2J", ""},
	} {
		t.Run(tc.body, func(t *testing.T) {
			style, ok := ansi.ParseCursorStyleReply(tc.body)
			if ok != (tc.sequence != "") {
				t.Fatalf("accepted = %t, want %t", ok, tc.sequence != "")
			}
			if ok && style.Sequence() != tc.sequence {
				t.Fatalf("restore = %q, want %q", style.Sequence(), tc.sequence)
			}
		})
	}
}

func TestAPieceReadsBackTheCursorStyleItSelects(t *testing.T) {
	for _, tc := range []struct {
		raw   string
		style ansi.CursorStyle
		ok    bool
	}{
		{"\x1b[ q", ansi.CursorReset, true},
		{"\x1b[0 q", ansi.CursorReset, true},
		{"\x1b[2 q", ansi.CursorSteadyBlock, true},
		{"\x1b[6 q", ansi.CursorSteadyBar, true},
		{"\x1b[7 q", 0, false},
		{"\x1b[?2 q", 0, false},
		{"\x1b[2;1 q", 0, false},
		{"\x1b[2q", 0, false},
		{"\x1b[2$q", 0, false},
	} {
		piece, _, ok := ansi.Next(tc.raw)
		if !ok {
			t.Fatalf("Next(%q) did not frame a piece", tc.raw)
		}
		if style, ok := piece.CursorStyle(); ok != tc.ok || style != tc.style {
			t.Errorf("CursorStyle(%q) = %v, %v; want %v, %v", tc.raw, style, ok, tc.style, tc.ok)
		}
	}
}
