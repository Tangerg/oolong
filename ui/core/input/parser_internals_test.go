package input

// Tests of what this package keeps to itself.
//
// The limits a parser puts on what it will hold are invariants rather than API:
// nothing outside can ask what they are, and a test that hard-coded the numbers
// would be asserting a coincidence.
// Everything else about this package is asserted from outside it, which is where
// a caller stands.

import (
	"strings"
	"testing"
)

func TestAnUnclosedPasteIsBounded(t *testing.T) {
	// A terminal that opens a paste and never closes it would otherwise swallow
	// everything typed afterwards into a buffer that only grows.
	var p Parser
	p.Feed([]byte("\x1b[200~"))
	var events []Event
	chunk := []byte(strings.Repeat("x", 1<<16))
	for range (maxPaste / len(chunk)) + 2 {
		events = append(events, p.Feed(chunk)...)
	}
	if len(events) == 0 {
		t.Fatal("an unbounded paste never delivered anything")
	}
	if !p.pasting {
		t.Fatal("the allocation bound ended paste mode before its closing sequence")
	}
	if len(p.paste) >= maxPaste {
		t.Fatalf("the pending paste retained %d bytes, want fewer than %d", len(p.paste), maxPaste)
	}
}

func TestALargePasteStaysTextUntilItsTerminator(t *testing.T) {
	var p Parser
	payload := strings.Repeat("x", maxPaste) + "\x1b[A" + strings.Repeat("y", 17)
	encoded := "\x1b[200~" + payload + "\x1b[201~"
	events := p.Feed([]byte(encoded))

	var pasted strings.Builder
	for i, event := range events {
		paste, ok := event.(Paste)
		if !ok {
			t.Fatalf("event %d is %T, want every chunk to remain a paste", i, event)
		}
		pasted.WriteString(paste.Text)
	}
	if got := pasted.String(); got != payload {
		t.Fatalf("joined paste has %d bytes, want the original %d", len(got), len(payload))
	}
	if p.pasting || p.paste != nil {
		t.Fatal("the closing sequence did not settle paste state")
	}
}

func TestRunawaySequenceIsDiscarded(t *testing.T) {
	// A stream that opens a sequence and never ends it must not grow the buffer
	// without limit.
	var p Parser
	p.Feed([]byte("\x1b[" + strings.Repeat("1;", maxSequenceBody)))
	if len(p.buf) > 0 {
		t.Fatal("a runaway sequence is still buffered")
	}

	// The sequence is still open, though, and the parser says so: something is
	// waiting on time rather than on more bytes. A byte that could end a control
	// sequence ends this one, because in the terminal's own stream that is what it
	// is — the keystroke it looks like never happened.
	if !p.Pending() {
		t.Fatal("the runaway sequence was forgotten before it ended")
	}
	if events := p.Feed([]byte("a")); len(events) != 0 {
		t.Fatalf("got %+v, want the final byte taken as the end of the sequence", events)
	}

	// And the parser works normally from the next byte on.
	events := p.Feed([]byte("a"))
	if len(events) != 1 {
		t.Fatalf("got %+v, want the parser to have recovered", events)
	}
}

func TestAFlushEndsARunawaySequence(t *testing.T) {
	// Otherwise the state outlives the stream that caused it, and the next
	// keystroke that happened to be a parameter byte would vanish into it.
	var p Parser
	p.Feed([]byte("\x1b[" + strings.Repeat("1;", maxSequenceBody)))
	p.Flush()
	if p.Pending() {
		t.Fatal("a flush left the runaway sequence open")
	}
	if events := p.Feed([]byte("0")); len(events) != 1 {
		t.Fatalf("got %+v, want an ordinary keystroke", events)
	}
}

func TestACodePointTooLargeToBeOneIsRefused(t *testing.T) {
	// A rune is 32 bits and a parsed number is not, so a conversion made before the
	// range is checked turns 0x100000041 into "A" — a key nobody pressed, arriving
	// because an integer wrapped.
	//
	// Two things stop it, and this pins both: parseParams refuses a number past
	// paramLimit, and codePoint refuses one past the last real code point. The first
	// is three functions away from the conversion, which is why the second exists —
	// an invariant nobody can see at the point that depends on it is an invariant
	// waiting to be moved.
	for _, seq := range []string{
		"\x1b[4294967361u",      // 0x100000041: narrows to 'A'
		"\x1b[4294967392u",      // 0x100000060: narrows to '`'
		"\x1b[1114112u",         // one past the last real code point
		"\x1b[97;1;4294967361u", // the same, in the associated-text group
	} {
		var p Parser
		for _, ev := range append(p.Feed([]byte(seq)), p.Flush()...) {
			key, ok := ev.(Key)
			if !ok {
				continue
			}
			if key.Rune == 'A' || key.Rune == '`' {
				t.Errorf("%q produced %q, which is what the number narrows to rather than what it says", seq, key.Rune)
			}
			if key.Text == "A" || key.Text == "`" {
				t.Errorf("%q produced text %q from a code point that is not one", seq, key.Text)
			}
		}
	}
}

func TestAnIdleParserHoldsNothing(t *testing.T) {
	var p Parser
	p.Feed([]byte("abc"))
	if p.buf != nil {
		t.Fatal("the buffer is still allocated after everything decoded")
	}
}
