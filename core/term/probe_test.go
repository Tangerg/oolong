package term_test

import (
	"os"
	"strings"
	"testing"
	"time"

	xterm "golang.org/x/term"

	"github.com/Tangerg/oolong/core/clipboard"
	"github.com/Tangerg/oolong/core/graphics"
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

	tty, err := term.OpenOn(replica, replica, term.Config{Features: term.Features{Probe: true}}, os.LookupEnv)
	if err != nil {
		t.Fatalf("opening a pty as a terminal: %v", err)
	}
	t.Cleanup(func() { _ = tty.Close() })
	return tty, primary
}

func TestProbeLearnsTheColoursTheTerminalDrawsWith(t *testing.T) {
	tty, _ := answered(t,
		"\x1b]11;rgb:1a1a/1b1b/2626\x07\x1b]10;rgb:c0c0/caca/f5f5\x07\x1b[?62;4;22c")

	ground := tty.Ground()
	if ground.BG.Default() {
		t.Fatal("the terminal said what it draws on and the session did not learn it")
	}
	if want := (grid.RGB{R: 0x1a, G: 0x1b, B: 0x26}); ground.BG.RGB() != want {
		t.Errorf("background = %+v, want %+v", ground.BG.RGB(), want)
	}
	if !ground.BG.RGB().Dark() {
		t.Error("a background of #1a1b26 was not taken as dark")
	}
	// The foreground is asked for so that a cell nobody coloured can still be
	// blended with — see [grid.Ground]. It is a separate answer and a separate
	// question, so a terminal that gives one and not the other is ordinary.
	if ground.FG.Default() {
		t.Fatal("the terminal said what it draws with and the session did not learn it")
	}
	if want := (grid.RGB{R: 0xc0, G: 0xca, B: 0xf5}); ground.FG.RGB() != want {
		t.Errorf("foreground = %+v, want %+v", ground.FG.RGB(), want)
	}
}

// TestProbeKeepsHalfAnAnswer because the two colour queries are two questions, and
// a terminal that answers one and ignores the other is commoner than one that
// answers neither.
func TestProbeKeepsHalfAnAnswer(t *testing.T) {
	tty, _ := answered(t, "\x1b]11;rgb:0/0/0\x07\x1b[?62;4;22c")

	ground := tty.Ground()
	if ground.BG.Default() {
		t.Error("the background the terminal did give was not kept")
	}
	if !ground.FG.Default() {
		t.Error("a foreground was reported by a terminal that never gave one")
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
		t.Errorf("features %v do not include sixel, which the terminal claimed", attrs.Features())
	}
	if attrs.Has(9) {
		t.Errorf("features %v include something the terminal never claimed", attrs.Features())
	}
}

// TestProbeStopsWaitingWhenTheTerminalSaysWhatItIs is the whole reason the
// attributes query is sent. A terminal that does not understand the colour query
// says nothing about it, and only the answer it always gives can distinguish that
// from an answer still on its way.
func TestProbeStopsWaitingWhenTheTerminalSaysWhatItIs(t *testing.T) {
	tty, _ := answered(t, "\x1b[?1;2c")

	if !tty.Ground().BG.Default() {
		t.Error("a background was reported by a terminal that never gave one")
	}
	if _, ok := tty.Attributes(); !ok {
		t.Error("the attributes the terminal did give were not kept")
	}
}

func TestProbeAsksNothingUnlessAsked(t *testing.T) {
	tty, primary := open(t, term.Config{})
	if !tty.Ground().BG.Default() {
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

	if !tty.Ground().BG.Default() {
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
	tty, primary := open(t, term.Config{})
	if !tty.Copy("hello") {
		t.Fatal("a small copy was refused")
	}
	want, _ := (&clipboard.Channel{}).Copy(clipboard.System, "hello")
	if got := read(t, primary, time.Second); !strings.Contains(got, want) {
		t.Errorf("the terminal was sent %q, which does not carry the copy", got)
	}
}

func TestCopyRefusesMoreThanItCanCarry(t *testing.T) {
	tty, _ := open(t, term.Config{})
	if tty.Copy(strings.Repeat("x", clipboard.Limit()+1)) {
		t.Error("a copy past the limit was reported as asked for")
	}
}

// TestPasteArrivesAsAPaste is the translation. Reading a clipboard and pasting into
// a terminal are the same event to whatever receives them, and this is the layer
// that knows the difference so that nothing above has to.
func TestPasteArrivesAsAPaste(t *testing.T) {
	tty, primary := open(t, term.Config{})
	<-tty.Events() // the opening size

	tty.Paste()
	want, _ := (&clipboard.Channel{}).Request(clipboard.System)
	if got := read(t, primary, time.Second); !strings.Contains(got, want) {
		t.Fatalf("the terminal was sent %q, which does not ask for the clipboard", got)
	}

	answer, _ := (&clipboard.Channel{}).Copy(clipboard.System, "from the clipboard")
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
	tty, primary := open(t, term.Config{})
	<-tty.Events()

	answer, _ := (&clipboard.Channel{}).Copy(clipboard.System, "unasked")
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
	tty, primary := open(t, term.Config{})
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

// TestGraphicsNeedsBothFacts is why the probe keeps the attributes. The environment
// names the terminal; only the terminal names sixel, so a terminal that draws sixel
// and nothing else is indistinguishable from one that draws nothing until it is
// asked.
func TestGraphicsNeedsBothFacts(t *testing.T) {
	for _, name := range []string{"KITTY_WINDOW_ID", "GHOSTTY_RESOURCES_DIR", "TERM_PROGRAM", "LC_TERMINAL"} {
		t.Setenv(name, "")
	}
	t.Setenv("TERM", "xterm")

	asked, _ := answered(t, "\x1b]11;rgb:0/0/0\x07\x1b[?62;4;22c")
	if got := asked.Graphics(); got != graphics.Sixel {
		t.Errorf("a terminal that claimed sixel reports %v", got)
	}

	// The same terminal, never asked, cannot know.
	unasked, _ := open(t, term.Config{})
	if got := unasked.Graphics(); got != graphics.None {
		t.Errorf("a terminal nobody asked reports %v, want none", got)
	}
}

func TestGraphicsPrefersAHandleOverAClaim(t *testing.T) {
	// A protocol that lets an image be named is worth more than one that only draws
	// pixels, however loudly the second is claimed.
	t.Setenv("TERM", "xterm-kitty")
	tty, _ := answered(t, "\x1b[?62;4c")
	if got := tty.Graphics(); got != graphics.Kitty {
		t.Errorf("= %v, want kitty", got)
	}
}

// TestTheTerminalNamesItself is the answer worth more than any variable. An
// environment describes the terminal a session started from; over ssh, in a container,
// or under a multiplexer that is not the one it is talking to.
func TestTheTerminalNamesItself(t *testing.T) {
	tty, _ := answered(t, "\x1bP>|kitty(0.32.2)\x1b\\\x1b[?62c")

	name, ok := tty.Name()
	if !ok {
		t.Fatal("the terminal named itself and the session did not learn it")
	}
	if name != "kitty(0.32.2)" {
		t.Errorf("name = %q", name)
	}
}

// TestTheNameDecidesTheProtocol, whatever the environment left behind.
func TestTheNameDecidesTheProtocol(t *testing.T) {
	for _, k := range []string{"KITTY_WINDOW_ID", "GHOSTTY_RESOURCES_DIR", "LC_TERMINAL"} {
		t.Setenv(k, "")
	}
	t.Setenv("TERM", "xterm-256color")
	t.Setenv("TERM_PROGRAM", "Apple_Terminal")

	tty, _ := answered(t, "\x1bP>|kitty(0.32.2)\x1b\\\x1b[?62c")
	if got := tty.Graphics(); got != graphics.Kitty {
		t.Errorf("graphics = %v, want kitty — the environment was stale", got)
	}
	if got := tty.Wheel(); got != (input.Wheel{Reports: 3, Rows: 3, Trackpad: 3}) {
		t.Errorf("wheel = %+v, want kitty's", got)
	}
}

func TestTheVersionNumberWhenThereIsNoName(t *testing.T) {
	// Alacritty exports no version and declines to name itself; this is what it does
	// answer.
	tty, _ := answered(t, "\x1b[>0;10000;1c\x1b[?62c")

	if _, ok := tty.Name(); ok {
		t.Error("a name was reported by a terminal that gave none")
	}
	version, ok := tty.Version()
	if !ok {
		t.Fatal("the version was not kept")
	}
	if version.Version != 10000 {
		t.Errorf("version = %+v", version)
	}
}

// TestWhichKeyboardEnhancementsTookEffect. Asking for them is not the same as getting
// them, and the difference is invisible in the events: a terminal that accepts
// disambiguation and gives nothing for releases leaves every key held for ever as far
// as this program can tell.
func TestWhichKeyboardEnhancementsTookEffect(t *testing.T) {
	// Only disambiguation, which is what an older Alacritty does.
	tty, _ := answered(t, "\x1b[?1u\x1b[?62c")

	flags, ok := tty.Keyboard()
	if !ok {
		t.Fatal("the terminal answered and the session did not learn it")
	}
	if !flags.Has(input.KeyboardDisambiguate) {
		t.Error("disambiguation was not reported")
	}
	if flags.Has(input.KeyboardReportEvents) {
		t.Error("releases were reported by a terminal that turned them off")
	}
}

func TestATerminalThatSaysNothingAboutItsKeyboard(t *testing.T) {
	tty, _ := answered(t, "\x1b[?62c")
	if _, ok := tty.Keyboard(); ok {
		t.Error("flags were reported by a terminal that said nothing")
	}
	if _, ok := tty.Name(); ok {
		t.Error("a name was reported by a terminal that said nothing")
	}
	if _, ok := tty.Version(); ok {
		t.Error("a version was reported by a terminal that said nothing")
	}
}

// TestNoAnswerIsReadAsTyping is the defect all of this was found by. Every one of
// these used to arrive as keystrokes — the keyboard flags as one invisible control
// character, the version string as fifteen.
func TestNoAnswerIsReadAsTyping(t *testing.T) {
	tty, _ := answered(t, "\x1bP>|kitty(0.32.2)\x1b\\\x1b[>0;276;0c\x1b[?31u\x1b[?62c!")

	// The only thing the session should see is the exclamation mark typed after them.
	for {
		ev, ok := next(t, tty)
		if !ok {
			t.Fatal("the keystroke never arrived")
		}
		if _, resize := ev.(input.Resize); resize {
			continue // the opening size, which every session is told
		}
		key, isKey := ev.(input.Key)
		if !isKey {
			t.Fatalf("an answer reached the session as %T %+v", ev, ev)
		}
		if !key.IsRune('!', 0) {
			t.Fatalf("an answer reached the session as the key %+v", key)
		}
		return
	}
}

func TestReportingTheDirectory(t *testing.T) {
	tty, primary := open(t, term.Config{})
	if err := tty.ReportDirectory("/tmp/some dir/x"); err != nil {
		t.Fatalf("ReportDirectory: %v", err)
	}
	got := read(t, primary, time.Second)
	if !strings.Contains(got, "\x1b]7;file://") {
		t.Errorf("the terminal was sent %q, which does not report a directory", got)
	}
	// The separators survive and the space does not, or no terminal resolves it.
	if !strings.Contains(got, "/tmp/some%20dir/x") {
		t.Errorf("the path came out as %q", got)
	}
}

func TestReportingTheDirectoryDefaultsToThisOne(t *testing.T) {
	tty, primary := open(t, term.Config{})
	if err := tty.ReportDirectory(""); err != nil {
		t.Fatalf("ReportDirectory: %v", err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Skip("no working directory")
	}
	if got := read(t, primary, time.Second); !strings.Contains(got, cwd) {
		t.Errorf("the terminal was sent %q, want the working directory %q", got, cwd)
	}
}

func TestAnAnswerToSomethingElseIsPassedOn(t *testing.T) {
	// A device control string that is not the version answers a question this session
	// did not ask, and is still addressed to the program.
	tty, _ := answered(t, "\x1bP1$r0 q\x1b\\\x1b[?62c")
	for {
		ev, ok := next(t, tty)
		if !ok {
			t.Fatal("the answer never arrived")
		}
		if dcs, isDCS := ev.(input.DCS); isDCS {
			if dcs.Body != "1$r0 q" {
				t.Errorf("body = %q", dcs.Body)
			}
			return
		}
	}
}

func TestReportingADirectoryThatCannotBeMadeAbsolute(t *testing.T) {
	// A path is made absolute because a relative one tells the terminal nothing it did
	// not already have.
	tty, primary := open(t, term.Config{})
	if err := tty.ReportDirectory("relative/path"); err != nil {
		t.Fatalf("ReportDirectory: %v", err)
	}
	got := read(t, primary, time.Second)
	if !strings.Contains(got, "/relative/path") || strings.Contains(got, "file://relative") {
		t.Errorf("the terminal was sent %q, want an absolute path", got)
	}
}
