package clipboard_test

import (
	"encoding/base64"
	"strings"
	"sync"
	"testing"
	"unicode/utf8"

	"github.com/Tangerg/oolong/core/clipboard"
)

func parameters(sequence string) (string, bool) {
	params, found := strings.CutPrefix(sequence, "\x1b]52;")
	return strings.TrimSuffix(params, "\x1b\\"), found
}

func answer(channel *clipboard.Channel, selection clipboard.Selection, params string) (string, bool) {
	if _, ok := channel.Request(selection); !ok {
		return "", false
	}
	return channel.Answer(params)
}

func TestCopyCarriesTheTextBackAgain(t *testing.T) {
	for _, value := range []string{
		"",
		"hello",
		"a line\nand another",
		"中文と日本語",
		"emoji 👩🏽‍🚀 with a zero-width joiner",
		"\x00\x01 control bytes",
		strings.Repeat("x", 4096),
	} {
		channel := &clipboard.Channel{}
		sequence, ok := channel.Copy(clipboard.System, value)
		if !ok {
			t.Errorf("%q was refused", value)
			continue
		}
		params, found := parameters(sequence)
		if !found {
			t.Errorf("%q produced %q, which is not an operating system command", value, sequence)
			continue
		}
		got, ok := answer(channel, clipboard.System, params)
		if value == "" {
			if ok {
				t.Errorf("an empty copy read back as %q", got)
			}
			continue
		}
		if !ok {
			t.Errorf("%q did not read back", value)
			continue
		}
		if got != value {
			t.Errorf("read back %q, want %q", got, value)
		}
	}
}

func TestCopyCannotEndItsOwnSequence(t *testing.T) {
	hostile := "innocent\x1b\\\x1b]52;c;" + base64.StdEncoding.EncodeToString([]byte("stolen")) +
		"\x1b\\ and \x07 a bell"

	sequence, ok := (&clipboard.Channel{}).Copy(clipboard.System, hostile)
	if !ok {
		t.Fatal("the text was refused rather than encoded")
	}
	body, found := parameters(sequence)
	if !found {
		t.Fatalf("copy sequence = %q", sequence)
	}
	_, body, _ = strings.Cut(body, ";")
	for _, forbidden := range []string{"\x1b", "\x07"} {
		if strings.Contains(body, forbidden) {
			t.Errorf("the payload contains %q, so the text can end the sequence early", forbidden)
		}
	}
}

func TestTheSelectionIsCarried(t *testing.T) {
	for _, test := range []struct {
		selection clipboard.Selection
		code      string
	}{{clipboard.System, "c"}, {clipboard.Primary, "p"}} {
		sequence, ok := (&clipboard.Channel{}).Copy(test.selection, "x")
		if !ok {
			t.Fatalf("selection %d was refused", test.selection)
		}
		want := "\x1b]52;" + test.code + ";"
		if !strings.HasPrefix(sequence, want) {
			t.Errorf("selection %d produced %q, want it to start %q", test.selection, sequence, want)
		}
	}
}

func TestAnUnknownSelectionBecomesTheSystemOne(t *testing.T) {
	sequence, ok := (&clipboard.Channel{}).Copy(clipboard.Selection(99), "x")
	if !ok {
		t.Fatal("refused")
	}
	if !strings.HasPrefix(sequence, "\x1b]52;c;") {
		t.Errorf("got %q, want it addressed to the system clipboard", sequence)
	}
}

func TestClearIsACopyOfNothing(t *testing.T) {
	if got, want := (&clipboard.Channel{}).Clear(clipboard.System), "\x1b]52;c;\x1b\\"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestNilChannelIsInert(t *testing.T) {
	var channel *clipboard.Channel
	if sequence, ok := channel.Copy(clipboard.System, "text"); ok || sequence != "" {
		t.Fatalf("nil copy = %q, %t", sequence, ok)
	}
	if sequence := channel.Clear(clipboard.System); sequence != "" {
		t.Fatalf("nil clear = %q", sequence)
	}
	if sequence, ok := channel.Request(clipboard.System); ok || sequence != "" {
		t.Fatalf("nil request = %q, %t", sequence, ok)
	}
	if text, ok := channel.Answer("c;dGV4dA=="); ok || text != "" {
		t.Fatalf("nil answer = %q, %t", text, ok)
	}
}

func TestRequestAsksOnceUntilItsAnswer(t *testing.T) {
	channel := &clipboard.Channel{}
	sequence, requested := channel.Request(clipboard.System)
	if !requested || sequence != "\x1b]52;c;?\x1b\\" {
		t.Fatalf("system request = %q, %t", sequence, requested)
	}
	if duplicate, accepted := channel.Request(clipboard.System); accepted || duplicate != "" {
		t.Fatalf("duplicate request = %q, %t", duplicate, accepted)
	}
	encoded := base64.StdEncoding.EncodeToString([]byte("text"))
	if _, accepted := channel.Answer("c;" + encoded); !accepted {
		t.Fatal("the request's answer was not accepted")
	}
	sequence, requested = channel.Request(clipboard.Primary)
	if !requested || sequence != "\x1b]52;p;?\x1b\\" {
		t.Fatalf("primary request = %q, %t", sequence, requested)
	}
}

func TestAnswerReadsWhatATerminalReturned(t *testing.T) {
	channel := &clipboard.Channel{}
	text, ok := answer(channel, clipboard.System,
		"c;"+base64.StdEncoding.EncodeToString([]byte("pasted")))
	if !ok || text != "pasted" {
		t.Fatalf("answer = %q, %t; want pasted", text, ok)
	}
}

func TestAnswerRefusesWhatItCannotRead(t *testing.T) {
	for _, params := range []string{
		"",
		"c",
		"c;",
		";aGk=",
		"cc;aGk=",
		"x;aGk=",
		"c;not base64!",
		"c;aGk",
	} {
		channel := &clipboard.Channel{}
		if text, ok := answer(channel, clipboard.System, params); ok {
			t.Errorf("%q was read as %q", params, text)
		}
	}
}

func TestAnswerRequiresTheRequestedSelection(t *testing.T) {
	channel := &clipboard.Channel{}
	if _, ok := channel.Request(clipboard.System); !ok {
		t.Fatal("request was refused")
	}
	primary := "p;" + base64.StdEncoding.EncodeToString([]byte("wrong"))
	if text, ok := channel.Answer(primary); ok {
		t.Fatalf("primary answer was accepted as %q for the system request", text)
	}
	system := "c;" + base64.StdEncoding.EncodeToString([]byte("right"))
	if text, ok := channel.Answer(system); !ok || text != "right" {
		t.Fatalf("matching answer = %q, %t", text, ok)
	}
}

func TestMalformedAnswersDoNotStealAValidRequest(t *testing.T) {
	channel := &clipboard.Channel{}
	if _, ok := channel.Request(clipboard.System); !ok {
		t.Fatal("request was refused")
	}
	for _, params := range []string{"", "c", "cc;aGk=", "x;aGk="} {
		if text, ok := channel.Answer(params); ok {
			t.Fatalf("malformed %q was accepted as %q", params, text)
		}
	}
	valid := "c;" + base64.StdEncoding.EncodeToString([]byte("still pending"))
	if text, ok := channel.Answer(valid); !ok || text != "still pending" {
		t.Fatalf("matching answer after malformed traffic = %q, %t", text, ok)
	}
}

func TestAnswerSanitizesWhatTheTerminalGaveBack(t *testing.T) {
	params := "c;" + base64.StdEncoding.EncodeToString([]byte{'a', 0xff, 0xfe, 'b'})
	text, ok := answer(&clipboard.Channel{}, clipboard.System, params)
	if !ok {
		t.Fatal("refused")
	}
	if !strings.ContainsRune(text, '�') {
		t.Errorf("text %q kept invalid UTF-8 instead of replacing it", text)
	}
}

func TestCopyRefusesMoreThanATerminalWillTake(t *testing.T) {
	channel := &clipboard.Channel{}
	if _, ok := channel.Copy(clipboard.System, strings.Repeat("x", clipboard.Limit())); !ok {
		t.Error("text exactly at the limit was refused")
	}
	if _, ok := channel.Copy(clipboard.System, strings.Repeat("x", clipboard.Limit()+1)); ok {
		t.Error("text past the limit was accepted")
	}
}

func TestAnswerRefusesContentPastTheLimit(t *testing.T) {
	over := base64.StdEncoding.EncodeToString([]byte(strings.Repeat("x", clipboard.Limit()+1)))
	if text, ok := answer(&clipboard.Channel{}, clipboard.System, "c;"+over); ok {
		t.Errorf("an answer of %d bytes was accepted", len(text))
	}

	absurd := strings.Repeat("A", base64.StdEncoding.EncodedLen(clipboard.Limit())+4)
	if text, ok := answer(&clipboard.Channel{}, clipboard.System, "c;"+absurd); ok {
		t.Errorf("an answer of %d bytes was accepted", len(text))
	}
}

func TestTmuxChannelWrapsOnlyItsOSC52Traffic(t *testing.T) {
	for name, env := range map[string]map[string]string{
		"session variable": {"TMUX": "/tmp/tmux-1000/default,1,0"},
		"terminal name":    {"TERM": "tmux-256color"},
	} {
		t.Run(name, func(t *testing.T) {
			channel := clipboard.New(func(name string) (string, bool) {
				value, ok := env[name]
				return value, ok
			})
			sequence, ok := channel.Copy(clipboard.System, "hello")
			if !ok {
				t.Fatal("copy was refused")
			}
			if !strings.HasPrefix(sequence, "\x1bPtmux;\x1b\x1b]52;c;") ||
				!strings.HasSuffix(sequence, "\x1b\x1b\\\x1b\\") {
				t.Fatalf("tmux copy sequence = %q", sequence)
			}
		})
	}
}

func TestOnlyOneConcurrentReadOwnsTheUnidentifiedAnswer(t *testing.T) {
	channel := &clipboard.Channel{}
	var wait sync.WaitGroup
	start := make(chan struct{})
	results := make(chan bool, 32)
	for range cap(results) {
		wait.Go(func() {
			<-start
			_, ok := channel.Request(clipboard.System)
			results <- ok
		})
	}
	close(start)
	wait.Wait()
	close(results)
	accepted := 0
	for ok := range results {
		if ok {
			accepted++
		}
	}
	if accepted != 1 {
		t.Fatalf("%d concurrent requests acquired one unidentified response, want 1", accepted)
	}
}

func FuzzAnswerNeverPanicsAndProducesOnlyValidText(f *testing.F) {
	for _, seed := range []string{
		"",
		"c;aGVsbG8=",
		"p;aGk=",
		"c;",
		"c;????",
		";;;;",
		"c;" + base64.StdEncoding.EncodeToString([]byte{0xff, 0xfe}),
		"\x00;\x00",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, params string) {
		selection := clipboard.System
		if strings.HasPrefix(params, "p;") {
			selection = clipboard.Primary
		}
		text, ok := answer(&clipboard.Channel{}, selection, params)
		if !ok {
			return
		}
		if !utf8.ValidString(text) {
			t.Fatalf("accepted %q, which is not valid UTF-8", text)
		}
		if len(text) > clipboard.Limit() {
			t.Fatalf("accepted %d bytes, past the limit of %d", len(text), clipboard.Limit())
		}
	})
}
