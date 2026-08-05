package term_test

import (
	"os"
	"strings"
	"testing"
	"time"

	xterm "golang.org/x/term"

	"github.com/Tangerg/oolong/core/clipboard"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/term"
)

// answered opens a terminal that has already been told what to say.
//
// A pty lets a test answer a question before it is asked: the bytes wait in the
// replica's input buffer until the probe reads them, so nothing has to be timed.
// Raw mode is set first because the line discipline would otherwise hold an escape
// sequence back, waiting for a newline it will never have.
func answered(t *testing.T, answer string) (*term.Terminal, *os.File) {
	t.Helper()
	primary, replica := pty(t)
	if _, err := xterm.MakeRaw(int(replica.Fd())); err != nil {
		t.Skipf("cannot put this pty in raw mode: %v", err)
	}
	if _, err := primary.WriteString(answer); err != nil {
		t.Fatalf("staging the terminal's answer: %v", err)
	}

	tty, err := term.OpenOn(replica, replica, term.Options{Probe: true})
	if err != nil {
		t.Fatalf("opening a pty as a terminal: %v", err)
	}
	t.Cleanup(func() { _ = tty.Close() })
	return tty, primary
}

func TestProbeLearnsTheBackgroundColour(t *testing.T) {
	tty, _ := answered(t, "\x1b]11;rgb:1a1a/1b1b/2626\x07\x1b[?62;4;22c")

	bg, ok := tty.Background()
	if !ok {
		t.Fatal("the terminal answered and the session did not learn it")
	}
	if want := (grid.RGB{R: 0x1a, G: 0x1b, B: 0x26}); bg != want {
		t.Errorf("background = %+v, want %+v", bg, want)
	}
	if !bg.Dark() {
		t.Error("a background of #1a1b26 was not taken as dark")
	}
}

func TestProbeLearnsWhatTheTerminalClaims(t *testing.T) {
	tty, _ := answered(t, "\x1b]11;rgb:0/0/0\x07\x1b[?62;4;22c")

	attrs, ok := tty.Attributes()
	if !ok {
		t.Fatal("the terminal said what it was and the session did not learn it")
	}
	if attrs.Class != 62 {
		t.Errorf("class = %d, want 62", attrs.Class)
	}
	if !attrs.Has(4) {
		t.Errorf("features %v do not include sixel, which the terminal claimed", attrs.Features)
	}
	if attrs.Has(9) {
		t.Errorf("features %v include something the terminal never claimed", attrs.Features)
	}
}

// TestProbeStopsWaitingWhenTheTerminalSaysWhatItIs is the whole reason the
// attributes query is sent. A terminal that does not understand the colour query
// says nothing about it, and only the answer it always gives can distinguish that
// from an answer still on its way.
func TestProbeStopsWaitingWhenTheTerminalSaysWhatItIs(t *testing.T) {
	tty, _ := answered(t, "\x1b[?1;2c")

	if _, ok := tty.Background(); ok {
		t.Error("a background was reported by a terminal that never gave one")
	}
	if _, ok := tty.Attributes(); !ok {
		t.Error("the attributes the terminal did give were not kept")
	}
}

func TestProbeAsksNothingUnlessAsked(t *testing.T) {
	tty, primary := open(t, term.Options{})
	if _, ok := tty.Background(); ok {
		t.Error("a terminal nobody questioned reported a background")
	}
	if _, ok := tty.Attributes(); ok {
		t.Error("a terminal nobody questioned reported its attributes")
	}

	// And nothing was asked of it, which is the observable half: a session that did
	// not ask must not be heard asking.
	written := read(t, primary, 200*time.Millisecond)
	for _, query := range []string{"\x1b]11;?", "\x1b[c"} {
		if strings.Contains(written, query) {
			t.Errorf("the terminal was asked %q anyway, in %q", query, written)
		}
	}
}

// next is the terminal's next event, or false when its input ended.
func next(t *testing.T, tty *term.Terminal) (input.Event, bool) {
	t.Helper()
	select {
	case ev, ok := <-tty.Events():
		return ev, ok
	case <-time.After(2 * time.Second):
		t.Fatal("no event arrived")
		return nil, false
	}
}

// TestProbeDoesNotEatWhatTheUserTyped covers the handover. The probe has to read
// the terminal before the pump exists, so anything it decodes that was not an
// answer belongs to the session and has to arrive there.
func TestProbeDoesNotEatWhatTheUserTyped(t *testing.T) {
	tty, _ := answered(t, "ab\x1b]11;rgb:0/0/0\x07\x1b[?62c\x1b[A")

	var keys []input.Key
	for len(keys) < 3 {
		ev, ok := next(t, tty)
		if !ok {
			t.Fatalf("input ended after %d keys: %+v", len(keys), keys)
		}
		if key, ok := ev.(input.Key); ok {
			keys = append(keys, key)
		}
	}
	if !keys[0].IsRune('a', 0) || !keys[1].IsRune('b', 0) {
		t.Errorf("the keys typed before the answer arrived as %+v", keys[:2])
	}
	if keys[2].Code != input.Up {
		t.Errorf("the key typed after the answer arrived as %+v", keys[2])
	}
}

// TestProbeHandsOverASequenceItSplit is the reason the parser is handed over and
// not just the events it produced. A sequence that straddles the moment the pump
// takes over still has to decode as one.
func TestProbeHandsOverASequenceItSplit(t *testing.T) {
	// The escape and bracket arrive with the answers; the rest comes afterwards.
	tty, primary := answered(t, "\x1b]11;rgb:0/0/0\x07\x1b[?62c\x1b[")
	if _, err := primary.WriteString("B"); err != nil {
		t.Fatalf("writing the rest of the sequence: %v", err)
	}

	for {
		ev, ok := next(t, tty)
		if !ok {
			t.Fatal("the sequence split across the handover never arrived")
		}
		if key, ok := ev.(input.Key); ok {
			if key.Code != input.Down {
				t.Fatalf("got %+v, want the Down key — the handover broke the sequence", key)
			}
			return
		}
	}
}

// TestProbeGivesUpOnATerminalThatSaysNothing is the backstop. A connection that
// filters the queries, or a terminal that answers neither, must not hold the start
// open indefinitely.
func TestProbeGivesUpOnATerminalThatSaysNothing(t *testing.T) {
	tty, _ := answered(t, "")

	if _, ok := tty.Background(); ok {
		t.Error("a background was reported by a terminal that said nothing")
	}
	if _, ok := tty.Attributes(); ok {
		t.Error("attributes were reported by a terminal that said nothing")
	}
}

// TestProbePassesOnAnAnswerToSomethingElse: an answer this session did not ask for
// is still addressed to the program rather than to the terminal, so a host that
// asked its own question earlier still gets its reply.
func TestProbePassesOnAnAnswerToSomethingElse(t *testing.T) {
	tty, _ := answered(t, "\x1b]52;c;aGk=\x07\x1b]11;rgb:0/0/0\x07\x1b[?62c")

	for {
		ev, ok := next(t, tty)
		if !ok {
			t.Fatal("the answer to the other question never arrived")
		}
		if osc, ok := ev.(input.OSC); ok {
			if osc.Command != 52 {
				t.Fatalf("got command %d, want 52", osc.Command)
			}
			if osc.Params != "c;aGk=" {
				t.Errorf("params = %q, want %q", osc.Params, "c;aGk=")
			}
			return
		}
	}
}

func TestCopyAsksTheTerminalToDoIt(t *testing.T) {
	// The terminal does the copying because over ssh, in a container, or through a
	// multiplexer running elsewhere it is the only end the user is at.
	tty, primary := open(t, term.Options{})
	if !tty.Copy("hello") {
		t.Fatal("a small copy was refused")
	}
	want, _ := clipboard.Copy(clipboard.System, "hello")
	if got := read(t, primary, time.Second); !strings.Contains(got, want) {
		t.Errorf("the terminal was sent %q, which does not carry the copy", got)
	}
}

func TestCopyRefusesMoreThanItCanCarry(t *testing.T) {
	tty, _ := open(t, term.Options{})
	if tty.Copy(strings.Repeat("x", clipboard.Limit()+1)) {
		t.Error("a copy past the limit was reported as asked for")
	}
}

// TestPasteArrivesAsAPaste is the translation. Reading a clipboard and pasting into
// a terminal are the same event to whatever receives them, and this is the layer
// that knows the difference so that nothing above has to.
func TestPasteArrivesAsAPaste(t *testing.T) {
	tty, primary := open(t, term.Options{})
	<-tty.Events() // the opening size

	tty.Paste()
	if got := read(t, primary, time.Second); !strings.Contains(got, clipboard.Request(clipboard.System)) {
		t.Fatalf("the terminal was sent %q, which does not ask for the clipboard", got)
	}

	answer, _ := clipboard.Copy(clipboard.System, "from the clipboard")
	if _, err := primary.WriteString(answer); err != nil {
		t.Fatal(err)
	}
	for {
		ev, ok := next(t, tty)
		if !ok {
			t.Fatal("the answer never arrived")
		}
		switch e := ev.(type) {
		case input.Paste:
			if e.Text != "from the clipboard" {
				t.Errorf("pasted %q", e.Text)
			}
			return
		case input.OSC:
			t.Fatalf("the answer arrived untranslated as %+v", e)
		}
	}
}

// TestAnAnswerNobodyAskedForIsNotAPaste. A terminal has no reason to volunteer one,
// but the alternative rule would let text arrive in a document nobody asked to put
// it in.
func TestAnAnswerNobodyAskedForIsNotAPaste(t *testing.T) {
	tty, primary := open(t, term.Options{})
	<-tty.Events()

	answer, _ := clipboard.Copy(clipboard.System, "unasked")
	if _, err := primary.WriteString(answer); err != nil {
		t.Fatal(err)
	}
	for {
		ev, ok := next(t, tty)
		if !ok {
			t.Fatal("the command never arrived at all")
		}
		switch e := ev.(type) {
		case input.OSC:
			if e.Command != 52 {
				t.Fatalf("got command %d, want 52", e.Command)
			}
			return
		case input.Paste:
			t.Fatalf("text nobody asked for arrived as the paste %q", e.Text)
		}
	}
}

// TestAnUnreadableAnswerIsNotAnEmptyPaste, because an empty paste would clear a
// selection the user still has.
func TestAnUnreadableAnswerIsNotAnEmptyPaste(t *testing.T) {
	tty, primary := open(t, term.Options{})
	<-tty.Events()

	tty.Paste()
	if _, err := primary.WriteString("\x1b]52;c;\x1b\\"); err != nil {
		t.Fatal(err)
	}
	for {
		ev, ok := next(t, tty)
		if !ok {
			t.Fatal("the answer never arrived")
		}
		switch e := ev.(type) {
		case input.OSC:
			return // passed through as what it is
		case input.Paste:
			t.Fatalf("an unreadable answer became the paste %q", e.Text)
		}
	}
}
