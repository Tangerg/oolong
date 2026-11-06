package clipboard_test

import (
	"context"
	"encoding/base64"
	"errors"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"
	"unicode/utf8"

	"github.com/Tangerg/oolong/ui/core/clipboard"
)

type readResult struct {
	text string
	err  error
}

func startRead(ctx context.Context, channel *clipboard.Channel, selection clipboard.Selection) (<-chan string, <-chan readResult) {
	sent := make(chan string, 1)
	result := make(chan readResult, 1)
	go func() {
		value, err := channel.Read(ctx, selection, func(sequence string) error {
			sent <- sequence
			return nil
		})
		result <- readResult{value, err}
	}()
	return sent, result
}

func parameters(sequence string) (string, bool) {
	params, found := strings.CutPrefix(sequence, "\x1b]52;")
	return strings.TrimSuffix(params, "\x1b\\"), found
}

func readAnswer(t *testing.T, channel *clipboard.Channel, selection clipboard.Selection, params string) readResult {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	sent, result := startRead(ctx, channel, selection)
	<-sent
	if !channel.Answer(params, time.Now()) {
		cancel()
	}
	return <-result
}

func TestCopyRoundTripsEveryPayloadWithoutTerminalCommands(t *testing.T) {
	for _, value := range []string{"", "hello", "a line\nand another", "中文と日本語", "emoji 👩🏽‍🚀", "\x00\x01", "\x1b\\\x1b]52;c;stolen\x07", strings.Repeat("x", 4096)} {
		channel := &clipboard.Channel{}
		sequence, ok := channel.Copy(clipboard.System, value)
		params, found := parameters(sequence)
		if !ok || !found {
			t.Fatalf("Copy(%q) = %q, %t", value, sequence, ok)
		}
		_, payload, _ := strings.Cut(params, ";")
		if strings.ContainsAny(payload, "\x1b\x07") {
			t.Fatalf("payload can escape its sequence: %q", payload)
		}
		got := readAnswer(t, channel, clipboard.System, params)
		if got.err != nil || got.text != value {
			t.Fatalf("round trip = %q, %v; want %q", got.text, got.err, value)
		}
	}
}

func TestReadSelectsTheRequestedClipboardAndKeepsOneWireOwner(t *testing.T) {
	for _, selection := range []clipboard.Selection{0, clipboard.System, clipboard.Primary} {
		channel := &clipboard.Channel{}
		sent, result := startRead(t.Context(), channel, selection)
		sequence := <-sent
		wire := selection
		if wire == 0 {
			wire = clipboard.System
		}
		if sequence != "\x1b]52;"+string(wire)+";?\x1b\\" {
			t.Fatalf("read sequence = %q", sequence)
		}
		if _, err := channel.Read(t.Context(), selection, func(string) error { t.Fatal("second query sent"); return nil }); !errors.Is(err, clipboard.ErrPending) {
			t.Fatalf("duplicate read = %v", err)
		}
		params := string(wire) + ";" + base64.StdEncoding.EncodeToString([]byte("answer"))
		if !channel.Answer(params, time.Now()) {
			t.Fatal("matching answer refused")
		}
		if got := <-result; got.err != nil || got.text != "answer" {
			t.Fatalf("read = %+v", got)
		}
		if channel.Answer(params, time.Now()) {
			t.Fatal("one response settled twice")
		}
		if got := readAnswer(t, channel, clipboard.Primary, "p;bmV4dA=="); got.err != nil || got.text != "next" {
			t.Fatalf("next query = %+v", got)
		}
	}
}

func TestUnrelatedRepliesCannotConsumeAQuery(t *testing.T) {
	channel := &clipboard.Channel{}
	before := time.Now().Add(-time.Second)
	sent, result := startRead(t.Context(), channel, clipboard.System)
	<-sent
	for _, params := range []string{"", "c", ";aGk=", "cc;aGk=", "x;aGk=", "p;aGk="} {
		if channel.Answer(params, time.Now()) {
			t.Fatalf("unrelated reply %q consumed query", params)
		}
	}
	if channel.Answer("c;aGk=", before) {
		t.Fatal("bytes received before admission consumed query")
	}
	if !channel.Answer("c;aGk=", time.Now()) {
		t.Fatal("matching reply did not settle")
	}
	if got := <-result; got.err != nil || got.text != "hi" {
		t.Fatalf("read = %+v", got)
	}
}

func TestInvalidPayloadSettlesWithAnExplicitFailure(t *testing.T) {
	for _, params := range []string{
		"c;not base64!", "c;aGk", "c;" + strings.Repeat("A", base64.StdEncoding.EncodedLen(clipboard.MaxPayload)+4),
		"c;" + base64.StdEncoding.EncodeToString([]byte(strings.Repeat("x", clipboard.MaxPayload+1))),
		"c;" + base64.StdEncoding.EncodeToString([]byte(strings.Repeat("\xffa", clipboard.MaxPayload/2))),
	} {
		channel := &clipboard.Channel{}
		if got := readAnswer(t, channel, clipboard.System, params); !errors.Is(got.err, clipboard.ErrResponse) || got.text != "" {
			t.Fatalf("invalid reply %q = %+v", params[:min(20, len(params))], got)
		}
		if got := readAnswer(t, channel, clipboard.System, "c;aGk="); got.err != nil || got.text != "hi" {
			t.Fatalf("invalid payload retained settled wire slot: %+v", got)
		}
	}
}

func TestRejectedReplyMatchesTheOutstandingSelectionAndAdmission(t *testing.T) {
	var absent *clipboard.Channel
	if absent.Reject("c;", time.Now()) {
		t.Fatal("nil channel accepted a rejection")
	}
	channel := &clipboard.Channel{}
	if channel.Reject("c;", time.Now()) {
		t.Fatal("unsolicited rejection consumed a query")
	}
	before := time.Now().Add(-time.Second)
	sent, result := startRead(t.Context(), channel, clipboard.Primary)
	<-sent
	for _, prefix := range []string{"", "p", ";", "pp;", "x;", "c;"} {
		if channel.Reject(prefix, time.Now()) {
			t.Fatalf("unrelated rejection %q consumed query", prefix)
		}
	}
	if channel.Reject("p;", before) {
		t.Fatal("rejection received before admission consumed query")
	}
	if !channel.Reject("p;", time.Now()) {
		t.Fatal("complete matching rejection did not settle")
	}
	if got := <-result; got.text != "" || !errors.Is(got.err, clipboard.ErrResponse) {
		t.Fatalf("rejection result = %+v", got)
	}
	if channel.Reject("p;", time.Now()) {
		t.Fatal("one rejection settled twice")
	}
	if got := readAnswer(t, channel, clipboard.System, "c;bmV3"); got.err != nil || got.text != "new" {
		t.Fatalf("rejection retained the answered wire slot: %+v", got)
	}
}

func TestRejectionUsesReceiveTimeWithoutRevivingRevokedDelivery(t *testing.T) {
	for _, outcome := range []string{"timely", "deadline", "cancelled"} {
		t.Run(outcome, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				channel := &clipboard.Channel{}
				sent, result := startRead(ctx, channel, clipboard.System)
				<-sent
				at := time.Now()
				want := clipboard.ErrResponse
				switch outcome {
				case "deadline":
					at = at.Add(10 * time.Second)
					want = context.DeadlineExceeded
				case "cancelled":
					cancel()
					synctest.Wait()
					want = context.Canceled
				}
				if !channel.Reject("c;", at) {
					t.Fatal("matching rejection was not consumed")
				}
				cancel()
				if got := <-result; got.text != "" || !errors.Is(got.err, want) {
					t.Fatalf("rejection result = %+v, want %v", got, want)
				}
				if got := readAnswer(t, channel, clipboard.System, "c;bmV3"); got.err != nil || got.text != "new" {
					t.Fatalf("consumed rejection retained wire slot: %+v", got)
				}
			})
		})
	}
}

func TestEmptyAndInvalidUTF8AnswersRemainData(t *testing.T) {
	for _, payload := range []string{"", "\n"} {
		if got := readAnswer(t, &clipboard.Channel{}, clipboard.System, "c;"+payload); got.err != nil || got.text != "" {
			t.Fatalf("empty payload = %+v", got)
		}
	}
	params := "c;" + base64.StdEncoding.EncodeToString([]byte{'a', 0xff, 0xfe, 'b'})
	if got := readAnswer(t, &clipboard.Channel{}, clipboard.System, params); got.err != nil || !utf8.ValidString(got.text) || !strings.ContainsRune(got.text, '�') {
		t.Fatalf("invalid UTF-8 = %+v", got)
	}
}

func TestReadRefusalDoesNotCreateAProtocolObligation(t *testing.T) {
	problem := errors.New("transport refused")
	channel := &clipboard.Channel{}
	if _, err := channel.Read(t.Context(), clipboard.System, func(string) error { return problem }); !errors.Is(err, problem) {
		t.Fatalf("transport error = %v", err)
	}
	if _, err := channel.Read(t.Context(), clipboard.System, nil); !errors.Is(err, errors.ErrUnsupported) {
		t.Fatalf("missing transport = %v", err)
	}
	if _, err := channel.Read(t.Context(), clipboard.Selection('x'), func(string) error { t.Fatal("invalid selection sent"); return nil }); !errors.Is(err, clipboard.ErrSelection) {
		t.Fatalf("invalid selection = %v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := channel.Read(ctx, clipboard.System, func(string) error { t.Fatal("cancelled query sent"); return nil }); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled admission = %v", err)
	}
	if channel.Answer("c;aGk=", time.Now()) {
		t.Fatal("refused query authorized unsolicited reply")
	}
	if got := readAnswer(t, channel, clipboard.System, "c;aGk="); got.err != nil {
		t.Fatalf("refused admission retained slot: %v", got.err)
	}
	var absent *clipboard.Channel
	if _, ok := absent.Copy(clipboard.System, "text"); ok {
		t.Fatal("nil copy accepted")
	}
	if _, err := absent.Read(t.Context(), clipboard.System, func(string) error { return nil }); !errors.Is(err, errors.ErrUnsupported) || absent.Answer("c;aGk=", time.Now()) {
		t.Fatalf("nil channel = %v", err)
	}
	absent.Interrupt()
}

func TestInterruptedAdmissionCannotForgetAnEscapedQuery(t *testing.T) {
	channel := &clipboard.Channel{}
	func() {
		defer func() {
			if recover() == nil {
				t.Error("send panic was hidden")
			}
		}()
		_, _ = channel.Read(t.Context(), clipboard.System, func(string) error { panic("after admission") })
	}()
	if _, err := channel.Read(t.Context(), clipboard.System, func(string) error { t.Fatal("uncertain query replaced"); return nil }); !errors.Is(err, clipboard.ErrPending) {
		t.Fatalf("uncertain admission lost its obligation: %v", err)
	}
	if !channel.Answer("c;b2xk", time.Now()) {
		t.Fatal("escaped query reply was not absorbed")
	}
	if got := readAnswer(t, channel, clipboard.System, "c;bmV3"); got.err != nil || got.text != "new" {
		t.Fatalf("new query = %+v", got)
	}
}

func TestCancellationAndHandoverRetainTheUnansweredWireSlot(t *testing.T) {
	for _, interrupt := range []bool{false, true} {
		channel := &clipboard.Channel{}
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		sent, result := startRead(ctx, channel, clipboard.System)
		<-sent
		want := context.Canceled
		if interrupt {
			channel.Interrupt()
			want = clipboard.ErrInterrupted
		} else {
			cancel()
		}
		if got := <-result; !errors.Is(got.err, want) || got.text != "" {
			t.Fatalf("revoked read = %+v", got)
		}
		if _, err := channel.Read(t.Context(), clipboard.System, func(string) error { t.Fatal("ambiguous retry sent"); return nil }); !errors.Is(err, clipboard.ErrPending) {
			t.Fatalf("retry = %v", err)
		}
		if !channel.Answer("c;b2xk", time.Now()) {
			t.Fatal("late reply was not absorbed")
		}
		if got := readAnswer(t, channel, clipboard.System, "c;bmV3"); got.err != nil || got.text != "new" {
			t.Fatalf("new read = %+v", got)
		}
	}
}

func TestTimeoutDoesNotLetAnOldReplyClaimTheNextRead(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		channel := &clipboard.Channel{}
		sent, result := startRead(t.Context(), channel, clipboard.System)
		<-sent
		at := time.Now()
		time.Sleep(11 * time.Second)
		if got := <-result; !errors.Is(got.err, context.DeadlineExceeded) {
			t.Fatalf("timeout = %+v", got)
		}
		if _, err := channel.Read(t.Context(), clipboard.System, func(string) error { t.Fatal("expired query replaced"); return nil }); !errors.Is(err, clipboard.ErrPending) {
			t.Fatalf("retry after timeout = %v", err)
		}
		// Even timely transport bytes decoded after settlement cannot revive delivery.
		if !channel.Answer("c;b2xk", at) {
			t.Fatal("old reply was not absorbed")
		}
		if got := readAnswer(t, channel, clipboard.System, "c;bmV3"); got.err != nil || got.text != "new" {
			t.Fatalf("next read = %+v", got)
		}
	})
}

func TestSettledReplySurvivesLaterCancellationAndDeliveryDelay(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		channel := &clipboard.Channel{}
		sent, result := startRead(ctx, channel, clipboard.System)
		<-sent
		if !channel.Answer("c;aGk=", time.Now()) {
			t.Fatal("timely reply refused")
		}
		cancel()
		time.Sleep(11 * time.Second)
		if got := <-result; got.err != nil || got.text != "hi" {
			t.Fatalf("settled result lost: %+v", got)
		}
	})
}

func TestOnlyOneConcurrentReaderCanAdmitAQuery(t *testing.T) {
	channel := &clipboard.Channel{}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	start := make(chan struct{})
	sent := make(chan struct{}, 32)
	results := make(chan error, 32)
	var workers sync.WaitGroup
	for range cap(results) {
		workers.Go(func() {
			<-start
			_, err := channel.Read(ctx, clipboard.System, func(string) error { sent <- struct{}{}; return nil })
			results <- err
		})
	}
	close(start)
	<-sent
	for range cap(results) - 1 {
		if err := <-results; !errors.Is(err, clipboard.ErrPending) {
			t.Fatalf("competing read = %v", err)
		}
	}
	channel.Answer("c;aGk=", time.Now())
	if err := <-results; err != nil {
		t.Fatalf("accepted read = %v", err)
	}
	workers.Wait()
	if len(sent) != 0 {
		t.Fatal("multiple queries reached transport")
	}
}

func TestCopyLimitsAndExplicitPassthrough(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		channel := clipboard.New(clipboard.Config{TmuxPassthrough: enabled})
		copySequence, copied := channel.Copy(clipboard.System, strings.Repeat("x", clipboard.MaxPayload))
		if !copied {
			t.Fatal("copy at limit refused")
		}
		if _, ok := channel.Copy(clipboard.System, strings.Repeat("x", clipboard.MaxPayload+1)); ok {
			t.Fatal("oversized copy accepted")
		}
		sent, result := startRead(t.Context(), channel, clipboard.System)
		readSequence := <-sent
		for _, sequence := range []string{copySequence, readSequence} {
			if enabled && (!strings.HasPrefix(sequence, "\x1bPtmux;\x1b\x1b]52;c;") || !strings.HasSuffix(sequence, "\x1b\x1b\\\x1b\\")) {
				t.Fatalf("invalid passthrough: %q", sequence[:min(50, len(sequence))])
			}
			if !enabled && !strings.HasPrefix(sequence, "\x1b]52;c;") {
				t.Fatalf("unexpected wrapper: %q", sequence[:min(50, len(sequence))])
			}
		}
		channel.Answer("c;aGk=", time.Now())
		<-result
	}
}

func FuzzAnswerNeverPanicsAndProducesOnlyValidText(f *testing.F) {
	for _, seed := range []string{"", "c;aGVsbG8=", "p;aGk=", "c;", "c;????", ";;;;", "c;//4=", "\x00;\x00"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, params string) {
		selection := clipboard.System
		if strings.HasPrefix(params, "p;") {
			selection = clipboard.Primary
		}
		got := readAnswer(t, &clipboard.Channel{}, selection, params)
		if got.err == nil && (!utf8.ValidString(got.text) || len(got.text) > clipboard.MaxPayload) {
			t.Fatalf("invalid result: %q", got.text)
		}
	})
}
