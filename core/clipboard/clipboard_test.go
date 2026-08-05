package clipboard_test

import (
	"encoding/base64"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/Tangerg/oolong/core/clipboard"
)

func TestCopyCarriesTheTextBackAgain(t *testing.T) {
	// The round trip is the only property worth asserting about the encoding: what
	// a terminal is handed is what a terminal would give back.
	for _, text := range []string{
		"",
		"hello",
		"a line\nand another",
		"中文と日本語",
		"emoji 👩🏽‍🚀 with a zero-width joiner",
		"\x00\x01 control bytes",
		strings.Repeat("x", 4096),
	} {
		seq, ok := clipboard.Copy(clipboard.System, text)
		if !ok {
			t.Errorf("%q was refused", text)
			continue
		}
		params, found := strings.CutPrefix(seq, "\x1b]52;")
		if !found {
			t.Errorf("%q produced %q, which is not an operating system command", text, seq)
			continue
		}
		params = strings.TrimSuffix(params, "\x1b\\")

		sel, got, ok := clipboard.Parse(params)
		if text == "" {
			// An empty copy is the sequence that clears, and its answer is the
			// terminal saying there is nothing there.
			if ok {
				t.Errorf("an empty copy read back as %q", got)
			}
			continue
		}
		if !ok {
			t.Errorf("%q did not read back", text)
			continue
		}
		if sel != clipboard.System {
			t.Errorf("%q read back on selection %q", text, rune(sel))
		}
		if got != text {
			t.Errorf("read back %q, want %q", got, text)
		}
	}
}

// TestCopyCannotEndItsOwnSequence is the reason the payload is encoded at all. Text
// that could close the sequence would have the rest of itself read as commands, and
// the text is the part that came from somewhere else.
func TestCopyCannotEndItsOwnSequence(t *testing.T) {
	hostile := "innocent\x1b\\\x1b]52;c;" + base64.StdEncoding.EncodeToString([]byte("stolen")) +
		"\x1b\\ and \x07 a bell"

	seq, ok := clipboard.Copy(clipboard.System, hostile)
	if !ok {
		t.Fatal("the text was refused rather than encoded")
	}
	body := strings.TrimSuffix(strings.TrimPrefix(seq, "\x1b]52;c;"), "\x1b\\")
	for _, forbidden := range []string{"\x1b", "\x07"} {
		if strings.Contains(body, forbidden) {
			t.Errorf("the payload contains %q, so the text can end the sequence early", forbidden)
		}
	}
}

func TestTheSelectionIsCarried(t *testing.T) {
	for _, sel := range []clipboard.Selection{clipboard.System, clipboard.Primary} {
		seq, ok := clipboard.Copy(sel, "x")
		if !ok {
			t.Fatalf("selection %q was refused", rune(sel))
		}
		if want := "\x1b]52;" + string(rune(sel)) + ";"; !strings.HasPrefix(seq, want) {
			t.Errorf("selection %q produced %q, want it to start %q", rune(sel), seq, want)
		}
	}
}

func TestAnUnknownSelectionBecomesTheSystemOne(t *testing.T) {
	// There are two, and a caller who passed something else meant the ordinary one.
	// Writing the byte through would produce a sequence the terminal cannot read.
	seq, ok := clipboard.Copy(clipboard.Selection('z'), "x")
	if !ok {
		t.Fatal("refused")
	}
	if !strings.HasPrefix(seq, "\x1b]52;c;") {
		t.Errorf("got %q, want it addressed to the system clipboard", seq)
	}
}

func TestClearIsACopyOfNothing(t *testing.T) {
	if got, want := clipboard.Clear(clipboard.System), "\x1b]52;c;\x1b\\"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRequestAsks(t *testing.T) {
	if got, want := clipboard.Request(clipboard.System), "\x1b]52;c;?\x1b\\"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if got, want := clipboard.Request(clipboard.Primary), "\x1b]52;p;?\x1b\\"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestParseReadsWhatATerminalAnswers(t *testing.T) {
	sel, text, ok := clipboard.Parse("c;" + base64.StdEncoding.EncodeToString([]byte("pasted")))
	if !ok {
		t.Fatal("a well-formed answer was refused")
	}
	if sel != clipboard.System {
		t.Errorf("selection = %q, want 'c'", rune(sel))
	}
	if text != "pasted" {
		t.Errorf("text = %q, want %q", text, "pasted")
	}
}

func TestParseRefusesWhatItCannotRead(t *testing.T) {
	// A refusal matters more here than in most places. An answer read as an empty
	// success would clear whatever the user had selected, so anything unreadable has
	// to be unreadable rather than empty.
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
		if sel, text, ok := clipboard.Parse(params); ok {
			t.Errorf("%q was read as selection %q and text %q", params, rune(sel), text)
		}
	}
}

func TestParseSanitizesWhatTheTerminalGaveBack(t *testing.T) {
	// The bytes came from a terminal, which got them from somewhere else again, and
	// they are about to become a string in a document and then in a cell.
	params := "c;" + base64.StdEncoding.EncodeToString([]byte{'a', 0xff, 0xfe, 'b'})
	_, text, ok := clipboard.Parse(params)
	if !ok {
		t.Fatal("refused")
	}
	if !strings.ContainsRune(text, '�') {
		t.Errorf("text %q kept invalid UTF-8 instead of replacing it", text)
	}
}

// TestCopyRefusesMoreThanATerminalWillTake: the far end's limit is silent, and it
// discards the whole payload rather than truncating. A refusal here is something a
// caller can explain; a copy that vanished is not.
func TestCopyRefusesMoreThanATerminalWillTake(t *testing.T) {
	if _, ok := clipboard.Copy(clipboard.System, strings.Repeat("x", clipboard.Limit())); !ok {
		t.Error("text exactly at the limit was refused")
	}
	if _, ok := clipboard.Copy(clipboard.System, strings.Repeat("x", clipboard.Limit()+1)); ok {
		t.Error("text past the limit was accepted")
	}
}

func TestParseRefusesAnAnswerTooLargeToBeOne(t *testing.T) {
	// Just over: this decodes to one byte past the limit, and only the exact check
	// after decoding can tell it from one exactly at the limit.
	over := base64.StdEncoding.EncodeToString([]byte(strings.Repeat("x", clipboard.Limit()+1)))
	if _, text, ok := clipboard.Parse("c;" + over); ok {
		t.Errorf("an answer of %d bytes was accepted", len(text))
	}

	// Far over: refused on its encoded length alone, without being decoded into
	// memory first.
	absurd := strings.Repeat("A", base64.StdEncoding.EncodedLen(clipboard.Limit())+4)
	if _, text, ok := clipboard.Parse("c;" + absurd); ok {
		t.Errorf("an answer of %d bytes was accepted", len(text))
	}
}

func FuzzParseNeverPanicsAndAnswersOnlyValidText(f *testing.F) {
	// Every one of these came off a terminal's input, which got it from somewhere
	// else again, and what comes out ends up in a document and then in a cell.
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
		sel, text, ok := clipboard.Parse(params)
		if !ok {
			return
		}
		if sel != clipboard.System && sel != clipboard.Primary {
			t.Fatalf("accepted selection %q, which is not one of the two", rune(sel))
		}
		if !utf8.ValidString(text) {
			t.Fatalf("accepted %q, which is not valid UTF-8", text)
		}
		if len(text) > clipboard.Limit() {
			t.Fatalf("accepted %d bytes, past the limit of %d", len(text), clipboard.Limit())
		}
	})
}
