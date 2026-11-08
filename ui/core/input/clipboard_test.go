package input_test

import (
	"context"
	"encoding/base64"
	"errors"
	"slices"
	"strings"
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

func startClipboardRead(ctx context.Context, t *testing.T, channel *clipboard.Channel) <-chan clipboardResult {
	t.Helper()
	started := make(chan struct{})
	result := make(chan clipboardResult, 1)
	go func() {
		text, err := channel.Read(ctx, clipboard.System, func(string) error { close(started); return nil })
		result <- clipboardResult{text, err}
	}()
	select {
	case <-started:
	case refused := <-result:
		t.Fatalf("clipboard read was refused: %+v", refused)
	}
	return result
}

func TestRequestedClipboardAnswerIsConsumedAndDeliveredToItsReader(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		channel := &clipboard.Channel{}
		result := startClipboardRead(t.Context(), t, channel)
		time.Sleep(time.Nanosecond)
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
	})
}

func TestClipboardFailuresAndEmptyAnswersCannotBecomePaste(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		for _, payload := range []string{"", "\n", "not-base64"} {
			channel := &clipboard.Channel{}
			result := startClipboardRead(t.Context(), t, channel)
			time.Sleep(time.Nanosecond)
			stream := input.NewStream(input.StreamConfig{Clipboard: channel})
			if events := stream.Feed([]byte("\x1b]52;c;"+payload+"\x1b\\"), time.Now()); len(events) != 0 {
				t.Fatalf("solicited empty/failure leaked into input: %+v", events)
			}
			got := <-result
			if got.text != "" || (payload == "not-base64") != errors.Is(got.err, clipboard.ErrResponse) {
				t.Fatalf("payload %q = %+v", payload, got)
			}
		}
	})
}

func TestExpiredClipboardReplyIsAbsorbedWithoutRevivingDelivery(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		channel := &clipboard.Channel{}
		result := startClipboardRead(t.Context(), t, channel)
		time.Sleep(time.Nanosecond)
		received := time.Now()
		time.Sleep(11 * time.Second)
		if got := <-result; !errors.Is(got.err, context.DeadlineExceeded) {
			t.Fatalf("timeout = %+v", got)
		}
		stream := input.NewStream(input.StreamConfig{Clipboard: channel})
		if events := stream.Feed([]byte("\x1b]52;c;cGFzdGVk\x1b\\"), received); len(events) != 0 {
			t.Fatalf("late decoded response became input: %+v", events)
		}
		result = startClipboardRead(t.Context(), t, channel)
		time.Sleep(time.Nanosecond)
		stream.Feed([]byte("\x1b]52;c;bmV3\x1b\\"), time.Now())
		if got := <-result; got.err != nil || got.text != "new" {
			t.Fatalf("new response = %+v", got)
		}
	})
}

func TestNativeBracketedPasteRemainsInputBesideAClipboardReply(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		channel := &clipboard.Channel{}
		result := startClipboardRead(t.Context(), t, channel)
		time.Sleep(time.Nanosecond)
		stream := input.NewStream(input.StreamConfig{Clipboard: channel})
		events := stream.Feed([]byte("\x1b]52;c;cXVlcnk=\x1b\\\x1b[200~native\x1b[201~"), time.Now())
		if len(events) != 1 || events[0] != (input.Paste{Text: "native"}) {
			t.Fatalf("native paste = %+v", events)
		}
		if got := <-result; got.err != nil || got.text != "query" {
			t.Fatalf("query result = %+v", got)
		}
	})
}

func TestOversizedClipboardRepliesSettleAcrossDecoderBoundaries(t *testing.T) {
	for _, size := range []int{base64.StdEncoding.EncodedLen(clipboard.MaxPayload) + 4, 1<<20 - 3, 1 << 20, 2 << 20} {
		for _, ending := range [][]string{{"\x07"}, {"\x1b", "\\"}} {
			synctest.Test(t, func(t *testing.T) {
				channel := &clipboard.Channel{}
				result := startClipboardRead(t.Context(), t, channel)
				time.Sleep(time.Nanosecond)
				stream := input.NewStream(input.StreamConfig{Clipboard: channel})
				payload := strings.Repeat("A", size)
				for _, chunk := range []string{"\x1b]", "52;", "c", ";", payload[:size/2], payload[size/2:]} {
					if events := stream.Feed([]byte(chunk), time.Now()); len(events) != 0 {
						t.Fatalf("unterminated reply of %d bytes produced input: %+v", size, events)
					}
				}
				requireClipboardPending(t, channel)
				select {
				case got := <-result:
					t.Fatalf("unterminated reply settled: %+v", got)
				default:
				}
				for _, prefix := range ending[:len(ending)-1] {
					if events := stream.Feed([]byte(prefix), time.Now()); len(events) != 0 {
						t.Fatalf("partial terminator produced input: %+v", events)
					}
					requireClipboardPending(t, channel)
				}
				at := time.Now()
				events := stream.Feed([]byte(ending[len(ending)-1]+"x\x1b[200~native\x1b[201~"), at)
				want := []input.Event{input.Key{Code: input.Character, Rune: 'x', At: at}, input.Paste{Text: "native"}}
				if !slices.Equal(events, want) {
					t.Fatalf("input after rejected reply = %+v, want %+v", events, want)
				}
				if got := <-result; got.text != "" || !errors.Is(got.err, clipboard.ErrResponse) {
					t.Fatalf("rejected reply of %d bytes = %+v", size, got)
				}
				requireNextClipboardRead(t, channel, stream)
			})
		}
	}
}

func TestUnrelatedRejectedOSCLeavesClipboardQueryPending(t *testing.T) {
	for _, tc := range []struct {
		name, intro, prefix string
		before              bool
	}{
		{name: "foreign selection", intro: "\x1b]52;p;", prefix: "p;"},
		{name: "unsupported selection", intro: "\x1b]52;x;", prefix: "x;"},
		{name: "long selection", intro: "\x1b]52;cc;", prefix: "cc;"},
		{name: "unfinished selection", intro: "\x1b]52;c"},
		{name: "another command", intro: "\x1b]11;c;", prefix: "c;"},
		{name: "received before admission", intro: "\x1b]52;c;", prefix: "c;", before: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				before := time.Now().Add(-time.Nanosecond)
				channel := &clipboard.Channel{}
				result := startClipboardRead(t.Context(), t, channel)
				time.Sleep(time.Nanosecond)
				stream := input.NewStream(input.StreamConfig{Clipboard: channel})
				at := time.Now()
				if tc.before {
					at = before
				}
				events := stream.Feed([]byte(tc.intro+strings.Repeat("A", 1<<20)+"\x1b\\"), at)
				if len(events) != 1 {
					t.Fatalf("unrelated rejection disappeared: %+v", events)
				}
				if rejection, ok := events[0].(input.OSCRejection); !ok || rejection.Prefix != tc.prefix {
					t.Fatalf("rejection = %+v, want prefix %q", events[0], tc.prefix)
				}
				requireClipboardPending(t, channel)
				stream.Feed([]byte("\x1b]52;c;aGk=\x07"), time.Now())
				if got := <-result; got.text != "hi" || got.err != nil {
					t.Fatalf("unrelated rejection changed the pending result: %+v", got)
				}
			})
		})
	}
}

func TestRejectedControlStringsCannotInventClipboardReplies(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		for _, intro := range []string{"\x1bP>|", "\x1b_Gi=1,I=1;"} {
			channel := &clipboard.Channel{}
			result := startClipboardRead(t.Context(), t, channel)
			time.Sleep(time.Nanosecond)
			stream := input.NewStream(input.StreamConfig{Clipboard: channel})
			if events := stream.Feed([]byte(intro+"c;"+strings.Repeat("A", 1<<20)+"\x1b\\"), time.Now()); len(events) != 0 {
				t.Fatalf("rejected non-OSC string produced input: %+v", events)
			}
			requireClipboardPending(t, channel)
			stream.Feed([]byte("\x1b]52;c;aGk=\x07"), time.Now())
			if got := <-result; got.text != "hi" || got.err != nil {
				t.Fatalf("non-OSC string settled clipboard: %+v", got)
			}
		}
	})
}

func TestUnterminatedOversizedClipboardReplyCannotReleaseTheWireSlot(t *testing.T) {
	for _, end := range []string{"EOF", "Escape", "expiry", "reset"} {
		t.Run(end, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				channel := &clipboard.Channel{}
				result := startClipboardRead(t.Context(), t, channel)
				time.Sleep(time.Nanosecond)
				stream := input.NewStream(input.StreamConfig{Clipboard: channel})
				at := time.Now()
				stream.Feed([]byte("\x1b]52;c;"+strings.Repeat("A", 1<<20)), at)
				var events []input.Event
				switch end {
				case "EOF":
					events = stream.Flush(at)
				case "Escape":
					events = stream.Feed([]byte("\x1b[B"), at)
				case "expiry":
					stream.Feed([]byte("\x1b"), at)
					events = stream.Expire(at.Add(input.DefaultEscapeTimeout))
				case "reset":
					stream.Reset()
				}
				for _, event := range events {
					if _, rejected := event.(input.OSCRejection); rejected {
						t.Fatalf("abandoned framing invented a complete rejection: %+v", events)
					}
				}
				requireClipboardPending(t, channel)
				stream.Feed([]byte("\x1b]52;c;aGk=\x07"), time.Now())
				if got := <-result; got.text != "hi" || got.err != nil {
					t.Fatalf("abandonment settled the clipboard result: %+v", got)
				}
			})
		})
	}
}

func TestLateOversizedClipboardReplyRetiresOnlyItsRevokedQuery(t *testing.T) {
	for _, end := range []string{"cancel", "timeout", "handover"} {
		t.Run(end, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				channel := &clipboard.Channel{}
				result := startClipboardRead(ctx, t, channel)
				time.Sleep(time.Nanosecond)
				stream := input.NewStream(input.StreamConfig{Clipboard: channel})
				received := time.Now()
				stream.Feed([]byte("\x1b]52;c;"+strings.Repeat("A", 1<<20)), received)
				want := context.Canceled
				switch end {
				case "cancel":
					cancel()
				case "timeout":
					time.Sleep(11 * time.Second)
					want = context.DeadlineExceeded
				case "handover":
					channel.Interrupt()
					want = clipboard.ErrInterrupted
				}
				if got := <-result; got.text != "" || !errors.Is(got.err, want) {
					t.Fatalf("revoked result = %+v, want %v", got, want)
				}
				requireClipboardPending(t, channel)
				if events := stream.Feed([]byte("\x1b\\"), received); len(events) != 0 {
					t.Fatalf("late rejection became input: %+v", events)
				}
				requireNextClipboardRead(t, channel, stream)
			})
		})
	}
}

func requireClipboardPending(t *testing.T, channel *clipboard.Channel) {
	t.Helper()
	_, err := channel.Read(t.Context(), clipboard.System, func(string) error {
		return errors.New("query admitted before the preceding reply ended")
	})
	if !errors.Is(err, clipboard.ErrPending) {
		t.Fatalf("unanswered clipboard slot = %v, want ErrPending", err)
	}
}

func requireNextClipboardRead(t *testing.T, channel *clipboard.Channel, stream *input.Stream) {
	t.Helper()
	result := startClipboardRead(t.Context(), t, channel)
	time.Sleep(time.Nanosecond)
	if events := stream.Feed([]byte("\x1b]52;c;bmV3\x07"), time.Now()); len(events) != 0 {
		t.Fatalf("next reply became input: %+v", events)
	}
	if got := <-result; got.text != "new" || got.err != nil {
		t.Fatalf("next clipboard read = %+v", got)
	}
}
