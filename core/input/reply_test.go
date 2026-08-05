package input_test

import (
	"testing"

	"github.com/Tangerg/oolong/core/input"
)

// TestAPrivateMarkerIsNeverAKeystroke is the rule, and the two below are what it was
// found by. A key never carries a private marker, so a sequence that does is a
// terminal answering — and reading one as a key puts whatever the terminal said into
// whatever had focus.
func TestAPrivateMarkerIsNeverAKeystroke(t *testing.T) {
	for _, in := range []string{
		"\x1b[?31u",       // the keyboard flags, which used to decode as U+001F
		"\x1b[?0u",        // the same, with nothing turned on
		"\x1b[?1;2;3;4$y", // a mode report, which nothing here asks for
		"\x1b[>0;276;0c",  // the version, which nothing used to read
		"\x1b[?2026$y",    // synchronised output's own report
		"\x1b[<1;2;3z",    // a mouse marker with a final byte that is not a mouse report
	} {
		for _, ev := range feed(in) {
			if key, ok := ev.(input.Key); ok {
				t.Errorf("%q decoded as the key %+v", in, key)
			}
		}
	}
}

// TestTheKeyboardFlagsRead is the answer to what a terminal actually turned on.
// Asking is not the same as being answered: a terminal may take the request for
// unambiguous codes and give nothing for releases, and nothing in the events
// themselves says so.
func TestTheKeyboardFlagsRead(t *testing.T) {
	got, ok := one(t, "\x1b[?31u").(input.KeyboardFlags)
	if !ok {
		t.Fatalf("did not decode as the flags")
	}
	if got.Flags != 31 {
		t.Errorf("flags = %d, want 31", got.Flags)
	}
	for _, flag := range []int{
		input.KittyDisambiguate, input.KittyReportEvents, input.KittyReportAlternates,
		input.KittyReportAllAsEscapes, input.KittyReportText,
	} {
		if !got.Has(flag) {
			t.Errorf("flag %d is not among %d", flag, got.Flags)
		}
	}

	// The case the whole thing exists for: the protocol is live and releases will
	// never come.
	partial := one(t, "\x1b[?1u").(input.KeyboardFlags)
	if !partial.Has(input.KittyDisambiguate) {
		t.Error("disambiguation was not reported")
	}
	if partial.Has(input.KittyReportEvents) {
		t.Error("releases were reported by a terminal that did not turn them on")
	}
}

func TestTheKeyboardFlagsOfNothing(t *testing.T) {
	got := one(t, "\x1b[?0u").(input.KeyboardFlags)
	if got.Flags != 0 {
		t.Errorf("flags = %d, want none", got.Flags)
	}
	if got.Has(input.KittyDisambiguate) {
		t.Error("a flag was reported when none were set")
	}
}

func TestTheVersionRead(t *testing.T) {
	got, ok := one(t, "\x1b[>0;276;0c").(input.DeviceVersion)
	if !ok {
		t.Fatal("did not decode as a version")
	}
	if got.Kind != 0 || got.Version != 276 || got.Patch != 0 {
		t.Errorf("got %+v, want kind 0 version 276 patch 0", got)
	}
	// The numbers come back as the terminal sent them, however it packs them.
	alacritty := one(t, "\x1b[>0;10000;1c").(input.DeviceVersion)
	if alacritty.Version != 10000 {
		t.Errorf("version = %d, want it unchanged", alacritty.Version)
	}
}

// TestTheVersionStringRead. It is the query the terminals that answer nothing else
// answer, and it used to arrive as Alt+Shift+P and fourteen more keystrokes.
func TestTheVersionStringRead(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"\x1bP>|kitty(0.32.2)\x1b\\", ">|kitty(0.32.2)"},
		{"\x1bP>|WezTerm 20240203\x1b\\", ">|WezTerm 20240203"},
		{"\x1bP1$r0 q\x1b\\", "1$r0 q"},
		{"\x1bP1+r544e=787465726d\x1b\\", "1+r544e=787465726d"},
	} {
		got, ok := one(t, tc.in).(input.DCS)
		if !ok {
			t.Errorf("%q did not decode as a device control string", tc.in)
			continue
		}
		if got.Body != tc.want {
			t.Errorf("%q gave %q, want %q", tc.in, got.Body, tc.want)
		}
	}
}

// TestTheVersionIntroducerIsAlsoAKeystroke, exactly as the other one is. Nothing a
// person types after Alt+Shift+P looks like a reply.
func TestTheVersionIntroducerIsAlsoAKeystroke(t *testing.T) {
	for _, in := range []string{
		"\x1bP",      // the chord alone
		"\x1bPa",     // and then a letter
		"\x1bP1x",    // a digit that is not followed by a marker
		"\x1bPhello", // ordinary typing
	} {
		var p input.Parser
		events := p.Feed([]byte(in))
		if len(events) == 0 {
			events = p.Flush()
		}
		if len(events) == 0 {
			t.Errorf("%q produced nothing", in)
			continue
		}
		key, ok := events[0].(input.Key)
		if !ok || !key.IsRune('P', input.Alt) {
			t.Errorf("%q produced %+v first, want alt+P", in, events[0])
		}
	}
}

func TestAVersionStringSplitAcrossReads(t *testing.T) {
	const reply = "\x1bP>|kitty(0.32.2)\x1b\\"
	for split := 1; split < len(reply); split++ {
		var p input.Parser
		events := append(p.Feed([]byte(reply[:split])), p.Feed([]byte(reply[split:]))...)
		if len(events) != 1 {
			t.Fatalf("split at %d produced %d events: %+v", split, len(events), events)
		}
		got, ok := events[0].(input.DCS)
		if !ok || got.Body != ">|kitty(0.32.2)" {
			t.Errorf("split at %d gave %+v", split, events[0])
		}
	}
}

func TestBothStringsShareTheirBound(t *testing.T) {
	// One accumulator, two introducers: the runaway rule is the same for both.
	for _, intro := range []string{"\x1b]52;", "\x1bP>|"} {
		var p input.Parser
		body := make([]byte, 1<<20+16)
		for i := range body {
			body[i] = 'A'
		}
		if got := p.Feed(append([]byte(intro), body...)); len(got) != 0 {
			t.Errorf("%q produced %d events for a runaway body", intro, len(got))
		}
		if !p.Pending() {
			t.Errorf("%q: Pending is false while the rest is still being dropped", intro)
		}
		if got := p.Feed([]byte("\x1b\\z")); len(got) != 1 {
			t.Errorf("%q: after the terminator, got %+v", intro, got)
		}
	}
}
