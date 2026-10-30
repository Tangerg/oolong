package input

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func TestPasteChunksPreservePayloadAndBound(t *testing.T) {
	for _, payload := range []string{strings.Repeat("\x1b", maxPaste+10), strings.Repeat("a", maxPaste-1) + "中文"} {
		var p Parser
		var got strings.Builder
		consume := func(events []Event) {
			for _, event := range events {
				paste, ok := event.(Paste)
				if !ok || !utf8.ValidString(paste.Text) || len(paste.Text) > maxPaste {
					t.Fatalf("invalid paste chunk: %T", event)
				}
				got.WriteString(paste.Text)
			}
			if len(p.paste) > maxPaste {
				t.Fatal("unbounded payload")
			}
		}
		consume(p.Feed([]byte("\x1b[200~")))
		for at := 0; at < len(payload); at += 997 {
			consume(p.Feed([]byte(payload[at:min(at+997, len(payload))])))
		}
		for _, b := range []byte("\x1b[201~") {
			consume(p.Feed([]byte{b}))
		}
		if got.String() != payload {
			t.Fatal("paste changed across chunks")
		}
	}
}

func TestEscapeExpiryPreservesIncompleteCharacter(t *testing.T) {
	var p Parser
	p.Feed([]byte{0xe4})
	if p.Ambiguous() {
		t.Fatal("UTF-8 is not Escape ambiguity")
	}
	p.Expire()
	events := p.Feed([]byte{0xb8, 0xad})
	if len(events) != 1 || events[0].(Key).Rune != '中' {
		t.Fatalf("events: %#v", events)
	}
}

func TestProtocolIdentity(t *testing.T) {
	for _, source := range []string{"\x1b[113;17u", "\x1b[113;33u", "\x1b[<128;1;1M", "\x1b[1 A", "\x1b[0u"} {
		var p Parser
		if events := p.Feed([]byte(source)); len(events) != 0 {
			t.Fatalf("%q: %#v", source, events)
		}
	}
	for _, source := range []string{"\x1b[0;;20013u", "\x1b[0;;" + strings.TrimSuffix(strings.Repeat("20013:", 100), ":") + "u"} {
		var p Parser
		events := p.Feed([]byte(source))
		if len(events) != 1 || events[0].(Key).Text == "" {
			t.Fatalf("missing associated text: %#v", events)
		}
	}
	var p Parser
	events := p.Feed([]byte("\x1b\x7f"))
	if len(events) != 1 || !events[0].(Key).Is(Backspace, Alt) {
		t.Fatalf("Alt+Backspace: %#v", events)
	}
}

func TestDefaultWheelReportsClassifyLikeExplicitDefault(t *testing.T) {
	var a Advance
	a.Wheel(Wheel{Trackpad: 15})
	now := time.Now()
	total := 0
	for i := range 3 {
		total += a.Rows(now.Add(time.Duration(i)*time.Millisecond), 1)
	}
	if total != 3 {
		t.Fatalf("rows=%d", total)
	}
}
