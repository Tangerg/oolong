package term_test

import (
	"strings"
	"testing"
	"time"

	xterm "golang.org/x/term"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/term"
)

func TestCloseRestoresTheReportedCursorStyle(t *testing.T) {
	tty, primary := answered(t, "\x1bP1$r4 q\x1b\\\x1b[?62c")
	if err := tty.Close(); err != nil {
		t.Fatal(err)
	}
	wire := read(t, primary, time.Second)
	if !strings.Contains(wire, "\x1bP$q q\x1b\\") {
		t.Fatalf("startup did not request cursor style: %q", wire)
	}
	if !strings.HasSuffix(wire, "\x1b[4 q\x1b[?25h") {
		t.Fatalf("reported steady underline was replaced by a protocol reset: %q", wire)
	}
}

func TestTransportModesUseTheConfiguredCursorRestorationTarget(t *testing.T) {
	modes := (term.Config{RestoreCursor: grid.CursorStyle{Shape: grid.CursorBar}}).Modes(nil)
	if got := modes.Leave(); !strings.HasSuffix(got, "\x1b[6 q\x1b[?25h") {
		t.Fatalf("restoration = %q", got)
	}
	if got := (term.Config{}).Modes(nil).Leave(); !strings.HasSuffix(got, "\x1b[0 q\x1b[?25h") {
		t.Fatalf("explicit protocol reset = %q", got)
	}
}

func TestLateCursorStatusCannotReplaceTheStartupRestorationTarget(t *testing.T) {
	tty, primary := answered(t, "\x1bP1$r2 q\x1b\\\x1b[?62c\x1bP1$r6 q\x1b\\")
	if err := tty.Close(); err != nil {
		t.Fatal(err)
	}
	if wire := read(t, primary, time.Second); !strings.HasSuffix(wire, "\x1b[2 q\x1b[?25h") {
		t.Fatalf("late reply changed entry style: %q", wire)
	}
}

func TestCursorRestorationUsesOnlyTheFirstValidStartupReply(t *testing.T) {
	for _, tc := range []struct {
		name, replies, want string
	}{
		{"unsupported", "\x1b[?62c", "\x1b[6 q\x1b[?25h"},
		{"rejected status", "\x1bP0$r4 q\x1b\\\x1b[?62c", "\x1b[6 q\x1b[?25h"},
		{"invalid shape", "\x1bP1$r9 q\x1b\\\x1b[?62c", "\x1b[6 q\x1b[?25h"},
		{"first valid shape", "\x1bP1$r2 q\x1b\\\x1bP1$r4 q\x1b\\\x1b[?62c", "\x1b[2 q\x1b[?25h"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			primary, replica := pty(t)
			if _, err := xterm.MakeRaw(int(replica.Fd())); err != nil {
				t.Fatal(err)
			}
			if _, err := primary.WriteString(tc.replies); err != nil {
				t.Fatal(err)
			}
			tty, err := term.OpenOn(replica, replica, term.Config{
				Features:      term.Features{Probe: true},
				RestoreCursor: grid.CursorStyle{Shape: grid.CursorBar},
			}, nil)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = tty.Close() })
			if err := tty.Close(); err != nil {
				t.Fatal(err)
			}
			if wire := read(t, primary, time.Second); !strings.HasSuffix(wire, tc.want) {
				t.Fatalf("restoration = %q, want suffix %q", wire, tc.want)
			}
		})
	}
}

func TestHandoverAndCloseShareTheEntryCursorStyle(t *testing.T) {
	tty, primary := answered(t, "\x1bP1$r4 q\x1b\\\x1b[?62c")
	if err := tty.Hand(nil, func() error {
		if wire := read(t, primary, time.Second); !strings.HasSuffix(wire, "\x1b[4 q\x1b[?25h") {
			t.Errorf("handover did not restore the entry cursor style: %q", wire)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := tty.Close(); err != nil {
		t.Fatal(err)
	}
	if wire := read(t, primary, time.Second); !strings.HasSuffix(wire, "\x1b[4 q\x1b[?25h") {
		t.Fatalf("close did not retain the entry cursor style: %q", wire)
	}
}
