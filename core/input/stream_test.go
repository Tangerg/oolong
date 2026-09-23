package input_test

import (
	"testing"
	"time"

	"github.com/Tangerg/oolong/core/clipboard"
	"github.com/Tangerg/oolong/core/input"
)

// What a transport is entitled to assume, stated once here because every transport
// now assumes it rather than deciding it again.

func TestTheZeroStreamIsADecoderOfItsOwn(t *testing.T) {
	// A transport with nothing to say about the parser, the grace or the clipboard
	// should not have to say it. Nothing here is constructed.
	at := time.Unix(1, 0)
	var stream input.Stream

	events := stream.Feed([]byte("a"), at)
	if len(events) != 1 {
		t.Fatalf("got %+v, want one key", events)
	}
	if key, ok := events[0].(input.Key); !ok || !key.At.Equal(at) {
		t.Fatalf("got %+v, want a key stamped %v", events[0], at)
	}
	stream.Feed([]byte{0x1b}, at)
	due, waiting := stream.DueAt()
	if !waiting || !due.Equal(at.Add(input.DefaultEscapeTimeout)) {
		t.Fatalf("due = %v %t, want the default grace", due, waiting)
	}
	if events := stream.Expire(due); len(events) != 1 {
		t.Fatalf("expiry produced %+v, want the Escape key", events)
	}
}

func TestALoneEscapeWaitsAndThenBecomesTheEscapeKey(t *testing.T) {
	at := time.Unix(1, 0)
	stream := input.NewStream(input.StreamConfig{Grace: 30 * time.Millisecond})

	if events := stream.Feed([]byte{0x1b}, at); len(events) != 0 {
		t.Fatalf("a lone escape decoded immediately as %+v", events)
	}
	due, waiting := stream.DueAt()
	if !waiting {
		t.Fatal("a lone escape left nothing for the driver to wake on")
	}
	if want := at.Add(30 * time.Millisecond); !due.Equal(want) {
		t.Fatalf("due at %v, want %v", due, want)
	}

	events := stream.Expire(due)
	if len(events) != 1 {
		t.Fatalf("expiry produced %+v, want the Escape key", events)
	}
	key, ok := events[0].(input.Key)
	if !ok || key.Code != input.Esc {
		t.Fatalf("expiry produced %+v, want the Escape key", events[0])
	}
	if _, waiting := stream.DueAt(); waiting {
		t.Fatal("the deadline survived the expiry that settled it")
	}
}

func TestBytesArrivingBeforeTheDeadlineSettleItInstead(t *testing.T) {
	at := time.Unix(1, 0)
	stream := input.NewStream(input.StreamConfig{})

	stream.Feed([]byte{0x1b}, at)
	if _, waiting := stream.DueAt(); !waiting {
		t.Fatal("a lone escape armed nothing")
	}
	events := stream.Feed([]byte("[A"), at.Add(time.Millisecond))
	if len(events) != 1 {
		t.Fatalf("the completed sequence produced %+v, want one key", events)
	}
	if _, waiting := stream.DueAt(); waiting {
		t.Fatal("the sequence completed and the deadline stayed armed")
	}
}

func TestAZeroGraceUsesTheDefaultEscapeTimeout(t *testing.T) {
	at := time.Unix(1, 0)
	stream := input.NewStream(input.StreamConfig{})
	stream.Feed([]byte{0x1b}, at)
	due, waiting := stream.DueAt()
	if !waiting || !due.Equal(at.Add(input.DefaultEscapeTimeout)) {
		t.Fatalf("due = %v %t, want %v", due, waiting, at.Add(input.DefaultEscapeTimeout))
	}
}

func TestEventsCarryWhenTheyArrived(t *testing.T) {
	at := time.Unix(1234, 0)
	stream := input.NewStream(input.StreamConfig{})
	events := stream.Feed([]byte("a"), at)
	if len(events) != 1 {
		t.Fatalf("got %+v, want one key", events)
	}
	key, ok := events[0].(input.Key)
	if !ok {
		t.Fatalf("got %+v, want a key", events[0])
	}
	if !key.At.Equal(at) {
		t.Fatalf("stamped %v, want %v", key.At, at)
	}
}

func TestAnAnsweredClipboardRequestBecomesAPaste(t *testing.T) {
	channel := &clipboard.Channel{}
	if _, ok := channel.Request(clipboard.System); !ok {
		t.Fatal("clipboard request was refused")
	}
	stream := input.NewStream(input.StreamConfig{Clipboard: channel})

	events := stream.Feed([]byte("\x1b]52;c;cGFzdGVk\x1b\\"), time.Unix(1, 0))
	if len(events) != 1 {
		t.Fatalf("got %+v, want one event", events)
	}
	paste, ok := events[0].(input.Paste)
	if !ok || paste.Text != "pasted" {
		t.Fatalf("got %+v, want the answer settled as a paste", events[0])
	}
}

func TestAnAnswerCarryingNoTextIsNotAPaste(t *testing.T) {
	// Base64 ignores line breaks, so a payload of one newline is a field with
	// something in it that decodes to nothing. Delivered as a paste, it replaces the
	// selection the user still has with an empty string.
	for _, answer := range []string{"\x1b]52;c;\x1b\\", "\x1b]52;c;\n\x1b\\"} {
		channel := &clipboard.Channel{}
		if _, ok := channel.Request(clipboard.System); !ok {
			t.Fatal("clipboard request was refused")
		}
		stream := input.NewStream(input.StreamConfig{Clipboard: channel})
		events := stream.Feed([]byte(answer), time.Unix(1, 0))
		if len(events) != 1 {
			t.Fatalf("%q got %+v, want one event", answer, events)
		}
		if paste, ok := events[0].(input.Paste); ok {
			t.Errorf("%q became a paste of %q", answer, paste.Text)
		}
	}
}

func TestAnUnaskedClipboardAnswerStaysTheCommandItWas(t *testing.T) {
	// A terminal has no reason to volunteer one, and text arriving in a document
	// nobody asked to put it in is not a thing to relax about.
	stream := input.NewStream(input.StreamConfig{Clipboard: &clipboard.Channel{}})
	events := stream.Feed([]byte("\x1b]52;c;cGFzdGVk\x1b\\"), time.Unix(1, 0))
	if len(events) != 1 {
		t.Fatalf("got %+v, want one event", events)
	}
	if _, ok := events[0].(input.OSC); !ok {
		t.Fatalf("got %+v, want the command passed through", events[0])
	}
}

func TestATransportWithoutAClipboardPassesTheCommandThrough(t *testing.T) {
	stream := input.NewStream(input.StreamConfig{})
	events := stream.Feed([]byte("\x1b]52;c;cGFzdGVk\x1b\\"), time.Unix(1, 0))
	if len(events) != 1 {
		t.Fatalf("got %+v, want one event", events)
	}
	if _, ok := events[0].(input.OSC); !ok {
		t.Fatalf("got %+v, want the command passed through", events[0])
	}
}

func TestAHandedOverParserKeepsTheSequenceItWasHoldingOnto(t *testing.T) {
	// A startup probe reads before the stream exists. What it was left holding is
	// ambiguous from the stream's first moment, with no feed to discover it.
	parser := &input.Parser{}
	if events := parser.Feed([]byte{0x1b}); len(events) != 0 {
		t.Fatalf("the probe's escape decoded as %+v", events)
	}
	at := time.Unix(1, 0)
	stream := input.NewStream(input.StreamConfig{Parser: parser, Grace: time.Second})

	if _, waiting := stream.DueAt(); waiting {
		t.Fatal("an unarmed stream reported a deadline")
	}
	stream.Arm(at)
	due, waiting := stream.DueAt()
	if !waiting || !due.Equal(at.Add(time.Second)) {
		t.Fatalf("due = %v %t, want %v", due, waiting, at.Add(time.Second))
	}
	if events := stream.Expire(due); len(events) != 1 {
		t.Fatalf("expiry produced %+v, want the Escape key", events)
	}
}

func TestFlushEndsTheStreamAndDisarmsIt(t *testing.T) {
	at := time.Unix(1, 0)
	stream := input.NewStream(input.StreamConfig{})
	stream.Feed([]byte{0x1b}, at)

	events := stream.Flush(at)
	if len(events) != 1 {
		t.Fatalf("flush produced %+v, want the Escape key", events)
	}
	if _, waiting := stream.DueAt(); waiting {
		t.Fatal("flush left a deadline nobody will reach")
	}
}
