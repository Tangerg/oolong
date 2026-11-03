package input_test

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"github.com/Tangerg/oolong/ui/core/clipboard"
	"github.com/Tangerg/oolong/ui/core/input"
)

type clipboardResult struct {
	text string
	err  error
}

func startClipboardRead(ctx context.Context, channel *clipboard.Channel) <-chan clipboardResult {
	started := make(chan struct{})
	result := make(chan clipboardResult, 1)
	go func() {
		text, err := channel.Read(ctx, clipboard.System, func(string) error { close(started); return nil })
		result <- clipboardResult{text, err}
	}()
	<-started
	return result
}

func TestRequestedClipboardAnswerIsConsumedAndDeliveredToItsReader(t *testing.T) {
	channel := &clipboard.Channel{}
	result := startClipboardRead(t.Context(), channel)
	stream := input.NewStream(input.StreamConfig{Clipboard: channel})
	events := stream.Feed([]byte("\x1b]52;c;cGFzdGVk\x1b\\x"), time.Now())
	if len(events) != 1 {
		t.Fatalf("solicited result leaked into input: %+v", events)
	}
	if key, ok := events[0].(input.Key); !ok || !key.IsRune('x', 0) {
		t.Fatalf("following key = %#v", events[0])
	}
	if got := <-result; got.err != nil || got.text != "pasted" {
		t.Fatalf("directed result = %+v", got)
	}
}

func TestClipboardFailuresAndEmptyAnswersCannotBecomePaste(t *testing.T) {
	for _, payload := range []string{"", "\n", "not-base64"} {
		channel := &clipboard.Channel{}
		result := startClipboardRead(t.Context(), channel)
		stream := input.NewStream(input.StreamConfig{Clipboard: channel})
		if events := stream.Feed([]byte("\x1b]52;c;"+payload+"\x1b\\"), time.Now()); len(events) != 0 {
			t.Fatalf("solicited empty/failure leaked into input: %+v", events)
		}
		got := <-result
		if got.text != "" || (payload == "not-base64") != errors.Is(got.err, clipboard.ErrResponse) {
			t.Fatalf("payload %q = %+v", payload, got)
		}
	}
}

func TestExpiredClipboardReplyIsAbsorbedWithoutRevivingDelivery(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		channel := &clipboard.Channel{}
		result := startClipboardRead(t.Context(), channel)
		received := time.Now()
		time.Sleep(11 * time.Second)
		if got := <-result; !errors.Is(got.err, context.DeadlineExceeded) {
			t.Fatalf("timeout = %+v", got)
		}
		stream := input.NewStream(input.StreamConfig{Clipboard: channel})
		if events := stream.Feed([]byte("\x1b]52;c;cGFzdGVk\x1b\\"), received); len(events) != 0 {
			t.Fatalf("late decoded response became input: %+v", events)
		}
		result = startClipboardRead(t.Context(), channel)
		stream.Feed([]byte("\x1b]52;c;bmV3\x1b\\"), time.Now())
		if got := <-result; got.err != nil || got.text != "new" {
			t.Fatalf("new response = %+v", got)
		}
	})
}

func TestNativeBracketedPasteRemainsInputBesideAClipboardReply(t *testing.T) {
	channel := &clipboard.Channel{}
	result := startClipboardRead(t.Context(), channel)
	stream := input.NewStream(input.StreamConfig{Clipboard: channel})
	events := stream.Feed([]byte("\x1b]52;c;cXVlcnk=\x1b\\\x1b[200~native\x1b[201~"), time.Now())
	if len(events) != 1 || events[0] != (input.Paste{Text: "native"}) {
		t.Fatalf("native paste = %+v", events)
	}
	if got := <-result; got.err != nil || got.text != "query" {
		t.Fatalf("query result = %+v", got)
	}
}
