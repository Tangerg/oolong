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
