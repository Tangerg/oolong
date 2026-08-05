package input_test

import (
	"image"
	"slices"
	"testing"

	"github.com/Tangerg/oolong/core/input"
)

// feed decodes a whole string in one go.
func feed(s string) []input.Event {
	var p input.Parser
	return p.Feed([]byte(s))
}

// one decodes a string that must produce exactly one event.
func one(t *testing.T, s string) input.Event {
	t.Helper()
	events := feed(s)
	if len(events) != 1 {
		t.Fatalf("decoding %q produced %d events, want 1: %+v", s, len(events), events)
	}
	return events[0]
}

func TestPlainCharacters(t *testing.T) {
	events := feed("ab中")
	want := []input.Key{
		{Code: input.Character, Rune: 'a'},
		{Code: input.Character, Rune: 'b'},
		{Code: input.Character, Rune: '中'},
	}
	if len(events) != len(want) {
		t.Fatalf("got %d events, want %d: %+v", len(events), len(want), events)
	}
	for i, w := range want {
		if got := events[i].(input.Key); got != w {
			t.Errorf("event %d = %+v, want %+v", i, got, w)
		}
	}
}

func TestNamedKeysAndControlChords(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want input.Key
	}{
		{"\r", input.Key{Code: input.Enter}},
		{"\t", input.Key{Code: input.Tab}},
		{"\x7f", input.Key{Code: input.Backspace}},
		{"\x08", input.Key{Code: input.Backspace}},
		{"\x03", input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl}},
		{"\x01", input.Key{Code: input.Character, Rune: 'a', Mods: input.Ctrl}},
		{"\x0a", input.Key{Code: input.Character, Rune: 'j', Mods: input.Ctrl}},
		// The C0 bytes that are not keys of their own. A decoder that dropped these
		// would make the chords unbindable, which is how input.Ctrl+Space stops working.
		{"\x00", input.Key{Code: input.Character, Rune: ' ', Mods: input.Ctrl}},
		{"\x1c", input.Key{Code: input.Character, Rune: '\\', Mods: input.Ctrl}},
		{"\x1d", input.Key{Code: input.Character, Rune: ']', Mods: input.Ctrl}},
		{"\x1e", input.Key{Code: input.Character, Rune: '^', Mods: input.Ctrl}},
		{"\x1f", input.Key{Code: input.Character, Rune: '_', Mods: input.Ctrl}},
	} {
		if got := one(t, tc.in).(input.Key); got != tc.want {
			t.Errorf("%q = %+v, want %+v", tc.in, got, tc.want)
		}
	}
}

func TestAltChords(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want input.Key
	}{
		{"\x1bx", input.Key{Code: input.Character, Rune: 'x', Mods: input.Alt}},
		{"\x1b中", input.Key{Code: input.Character, Rune: '中', Mods: input.Alt}},
		{"\x1b\r", input.Key{Code: input.Enter, Mods: input.Alt}},
	} {
		if got := one(t, tc.in).(input.Key); got != tc.want {
			t.Errorf("%q = %+v, want %+v", tc.in, got, tc.want)
		}
	}
}

func TestCursorAndFunctionKeys(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want input.Key
	}{
		{"\x1b[A", input.Key{Code: input.Up}},
		{"\x1b[B", input.Key{Code: input.Down}},
		{"\x1b[C", input.Key{Code: input.Right}},
		{"\x1b[D", input.Key{Code: input.Left}},
		{"\x1b[H", input.Key{Code: input.Home}},
		{"\x1b[F", input.Key{Code: input.End}},
		{"\x1b[1;5A", input.Key{Code: input.Up, Mods: input.Ctrl}},
		{"\x1b[1;2D", input.Key{Code: input.Left, Mods: input.Shift}},
		{"\x1b[1;3B", input.Key{Code: input.Down, Mods: input.Alt}},
		{"\x1b[1;8C", input.Key{Code: input.Right, Mods: input.Shift | input.Alt | input.Ctrl}},
		{"\x1b[Z", input.Key{Code: input.Backtab, Mods: input.Shift}},
		{"\x1b[3~", input.Key{Code: input.Delete}},
		{"\x1b[5~", input.Key{Code: input.PageUp}},
		{"\x1b[6~", input.Key{Code: input.PageDown}},
		{"\x1b[2~", input.Key{Code: input.Insert}},
		{"\x1b[1~", input.Key{Code: input.Home}},
		{"\x1b[4~", input.Key{Code: input.End}},
		{"\x1b[15~", input.Key{Code: input.F5}},
		{"\x1b[24~", input.Key{Code: input.F12}},
		{"\x1b[15;5~", input.Key{Code: input.F5, Mods: input.Ctrl}},
		{"\x1bOP", input.Key{Code: input.F1}},
		{"\x1bOA", input.Key{Code: input.Up}},
		{"\x1b[P", input.Key{Code: input.F1}},
		{"\x1b[S", input.Key{Code: input.F4}},
	} {
		if got := one(t, tc.in).(input.Key); got != tc.want {
			t.Errorf("%q = %+v, want %+v", tc.in, got, tc.want)
		}
	}
}

func TestExtendedKeyReports(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   string
		want input.Key
	}{
		{"plain letter", "\x1b[97u", input.Key{Code: input.Character, Rune: 'a'}},
		{"with modifier", "\x1b[97;5u", input.Key{Code: input.Character, Rune: 'a', Mods: input.Ctrl}},
		{"repeat", "\x1b[97;1:2u", input.Key{Code: input.Character, Rune: 'a', Transition: input.Repeat}},
		{"release", "\x1b[97;1:3u", input.Key{Code: input.Character, Rune: 'a', Transition: input.Release}},
		{"named key", "\x1b[57352u", input.Key{Code: input.Up}},
		{"function key", "\x1b[57364u", input.Key{Code: input.F1}},
		{"super modifier", "\x1b[97;9u", input.Key{Code: input.Character, Rune: 'a', Mods: input.Super}},
		{"associated text", "\x1b[97;1;98u", input.Key{Code: input.Character, Rune: 'a', Text: "b"}},
		{"alternate codes accepted", "\x1b[97:65:97u", input.Key{Code: input.Character, Rune: 'a'}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := one(t, tc.in).(input.Key); got != tc.want {
				t.Errorf("%q = %+v, want %+v", tc.in, got, tc.want)
			}
		})
	}
}

func TestExtendedKeyReportsThatMustBeRefused(t *testing.T) {
	// A key event with the wrong modifiers fires something the user did not ask
	// for, so a report that cannot be trusted produces nothing at all.
	for _, in := range []string{
		"\x1b[u",               // a cursor report, not a key
		"\x1b[0u",              // no key has code zero
		"\x1b[97;99999999999u", // a modifier parameter beyond any encoding
		"\x1b[97;1:9u",         // an event type this protocol does not define
		"\x1b[97;1:2:3u",       // too many subparameters
		"\x1b[57400u",          // a private-use number this program does not know
		"\x1b[97;1;2;3u",       // more parameter groups than the report has
		"\x1b[97;1;1114112u",   // associated text outside Unicode
	} {
		if events := feed(in); len(events) != 0 {
			t.Errorf("%q produced %+v, want nothing", in, events)
		}
	}
}

func TestMouseReports(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   string
		want input.Mouse
	}{
		{"left press", "\x1b[<0;10;5M", input.Mouse{Pos: image.Pt(9, 4), Action: input.MouseDown, Button: input.ButtonLeft}},
		{"left release", "\x1b[<0;10;5m", input.Mouse{Pos: image.Pt(9, 4), Action: input.MouseUp, Button: input.ButtonLeft}},
		{"middle press", "\x1b[<1;1;1M", input.Mouse{Pos: image.Pt(0, 0), Action: input.MouseDown, Button: input.ButtonMiddle}},
		{"right press", "\x1b[<2;1;1M", input.Mouse{Pos: image.Pt(0, 0), Action: input.MouseDown, Button: input.ButtonRight}},
		{"drag", "\x1b[<32;3;4M", input.Mouse{Pos: image.Pt(2, 3), Action: input.MouseDrag, Button: input.ButtonLeft}},
		{"move", "\x1b[<35;3;4M", input.Mouse{Pos: image.Pt(2, 3), Action: input.MouseMove}},
		{"wheel up", "\x1b[<64;3;4M", input.Mouse{Pos: image.Pt(2, 3), Action: input.WheelUp}},
		{"wheel down", "\x1b[<65;3;4M", input.Mouse{Pos: image.Pt(2, 3), Action: input.WheelDown}},
		{"shift and ctrl", "\x1b[<20;1;1M", input.Mouse{Pos: image.Pt(0, 0), Action: input.MouseDown, Button: input.ButtonLeft, Mods: input.Shift | input.Ctrl}},
		{"every modifier", "\x1b[<28;1;1M", input.Mouse{Pos: image.Pt(0, 0), Action: input.MouseDown, Button: input.ButtonLeft, Mods: input.Shift | input.Alt | input.Ctrl}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := one(t, tc.in).(input.Mouse); got != tc.want {
				t.Errorf("%q = %+v, want %+v", tc.in, got, tc.want)
			}
		})
	}
}

func TestMouseReportsThatMustBeRefused(t *testing.T) {
	for _, in := range []string{
		"\x1b[<0;10M",         // no row
		"\x1b[<0;99999999;5M", // a column beyond any encoding
		"\x1b[<66;1;1M",       // the horizontal wheel, which nothing here reads
		"\x1b[<99999999;1;1M", // a button field beyond any encoding
	} {
		if events := feed(in); len(events) != 0 {
			t.Errorf("%q produced %+v, want nothing", in, events)
		}
	}
}

func TestFocusReports(t *testing.T) {
	if _, ok := one(t, "\x1b[I").(input.FocusIn); !ok {
		t.Error("CSI I did not decode as focus gained")
	}
	if _, ok := one(t, "\x1b[O").(input.FocusOut); !ok {
		t.Error("CSI O did not decode as focus lost")
	}
}

func TestPaste(t *testing.T) {
	ev := one(t, "\x1b[200~hello world\x1b[201~")
	paste, ok := ev.(input.Paste)
	if !ok {
		t.Fatalf("event = %T, want input.Paste", ev)
	}
	if paste.Text != "hello world" {
		t.Fatalf("pasted %q", paste.Text)
	}
}

func TestPasteKeepsWhatWouldOtherwiseBeInterpreted(t *testing.T) {
	// The whole point of a bracketed paste: what is inside is text, even when it
	// looks like keys. Interpreting it is how pasting code runs commands.
	ev := one(t, "\x1b[200~line\rnext\ttab\x1b[Anot-a-key\x1b[201~")
	paste := ev.(input.Paste)
	if paste.Text != "line\rnext\ttab\x1b[Anot-a-key" {
		t.Fatalf("pasted %q, want the bytes verbatim", paste.Text)
	}
}

func TestPasteSplitAcrossReads(t *testing.T) {
	var p input.Parser
	if events := p.Feed([]byte("\x1b[200~part one ")); len(events) != 0 {
		t.Fatalf("an unfinished paste produced %+v", events)
	}
	// Split inside the closing sequence, which is where a naive decoder loses the
	// paste or emits its terminator as keys.
	if events := p.Feed([]byte("part two\x1b[20")); len(events) != 0 {
		t.Fatalf("a split terminator produced %+v", events)
	}
	events := p.Feed([]byte("1~"))
	if len(events) != 1 {
		t.Fatalf("got %+v, want the completed paste", events)
	}
	if got := events[0].(input.Paste).Text; got != "part one part two" {
		t.Fatalf("pasted %q", got)
	}
}

func TestFlushDoesNotCutAPasteShort(t *testing.T) {
	var p input.Parser
	p.Feed([]byte("\x1b[200~half"))
	// A paste is incomplete, not ambiguous: forcing it would corrupt the text.
	if events := p.Flush(); len(events) != 0 {
		t.Fatalf("Flush produced %+v mid-paste", events)
	}
	events := p.Feed([]byte(" whole\x1b[201~"))
	if got := events[0].(input.Paste).Text; got != "half whole" {
		t.Fatalf("pasted %q", got)
	}
}

func TestStrayPasteTerminatorIsIgnored(t *testing.T) {
	if events := feed("\x1b[201~"); len(events) != 0 {
		t.Fatalf("a terminator with no paste open produced %+v", events)
	}
}

func TestLoneEscapeResolvesOnlyOnFlush(t *testing.T) {
	var p input.Parser
	if events := p.Feed([]byte("\x1b")); len(events) != 0 {
		t.Fatalf("a lone escape decoded immediately as %+v", events)
	}
	if !p.Pending() {
		t.Fatal("Pending does not report the buffered escape, so no timer would be armed")
	}
	events := p.Flush()
	if len(events) != 1 || events[0].(input.Key).Code != input.Esc {
		t.Fatalf("Flush produced %+v, want the Escape key", events)
	}
	if p.Pending() {
		t.Fatal("something is still buffered after Flush")
	}
}

func TestAnIntroducerThatNeverCompletedIsAChord(t *testing.T) {
	// An introducer is also the character a terminal sends for Alt with that key,
	// and by the time the wait is over the two are no longer ambiguous: both bytes
	// arrived before the pause, so they came in one burst. A burst is what a chord
	// is — an escape the user pressed on its own is followed by a human gap, and
	// resolves as Escape while nothing has followed it at all.
	//
	// Getting this wrong is what makes Alt+[, Alt+] and Alt+Shift+O unbindable.
	for _, tc := range []struct {
		in   string
		want rune
	}{
		{"\x1b[", '['},
		{"\x1b]", ']'},
		{"\x1bO", 'O'},
	} {
		var p input.Parser
		p.Feed([]byte(tc.in))
		events := p.Flush()
		if len(events) != 1 {
			t.Fatalf("%q produced %+v, want one chord", tc.in, events)
		}
		if got := events[0].(input.Key); !got.IsRune(tc.want, input.Alt) {
			t.Errorf("%q produced %+v, want alt+%c", tc.in, got, tc.want)
		}
	}
}

func TestEscapeThenTypingIsTwoKeystrokes(t *testing.T) {
	// The other half: the user pressed Escape, the wait ran out, and only then did
	// they type. The gap is the whole difference, and the parser sees it as one.
	var p input.Parser
	p.Feed([]byte("\x1b"))
	events := p.Flush()
	if len(events) != 1 || events[0].(input.Key).Code != input.Esc {
		t.Fatalf("got %+v, want the Escape key", events)
	}
	events = p.Feed([]byte("["))
	if len(events) != 1 {
		t.Fatalf("got %+v, want the bracket", events)
	}
	if got := events[0].(input.Key); !got.IsRune('[', 0) {
		t.Errorf("got %+v, want a bare bracket", got)
	}
}

func TestTwoEscapesInARow(t *testing.T) {
	events := feed("\x1b\x1b")
	if len(events) != 1 || events[0].(input.Key).Code != input.Esc {
		t.Fatalf("got %+v, want one Escape with the second still buffered", events)
	}
}

func TestSequencesSplitAtEveryBoundary(t *testing.T) {
	// A read can land anywhere. Every split of a sequence has to decode to the
	// same event as the whole, or keys are lost under load.
	const seq = "\x1b[1;5A"
	for split := 1; split < len(seq); split++ {
		var p input.Parser
		events := append(p.Feed([]byte(seq[:split])), p.Feed([]byte(seq[split:]))...)
		if len(events) != 1 {
			t.Fatalf("split at %d produced %+v, want one event", split, events)
		}
		if got := events[0].(input.Key); got != (input.Key{Code: input.Up, Mods: input.Ctrl}) {
			t.Fatalf("split at %d produced %+v", split, got)
		}
	}
}

func TestOneByteAtATime(t *testing.T) {
	const in = "a\x1b[B\x1b[<0;2;3M\x1b[200~p\x1b[201~"
	var p input.Parser
	var events []input.Event
	for i := range len(in) {
		events = append(events, p.Feed([]byte{in[i]})...)
	}
	events = append(events, p.Flush()...)
	if len(events) != 4 {
		t.Fatalf("got %d events, want 4: %+v", len(events), events)
	}
	if got := events[0].(input.Key); !got.IsRune('a', 0) {
		t.Fatalf("first = %+v", got)
	}
	if got := events[1].(input.Key).Code; got != input.Down {
		t.Fatalf("second = %v", got)
	}
	if got := events[2].(input.Mouse).Pos; got != image.Pt(1, 2) {
		t.Fatalf("third = %v", got)
	}
	if got := events[3].(input.Paste).Text; got != "p" {
		t.Fatalf("fourth = %q", got)
	}
}

func TestCharacterSplitAcrossReads(t *testing.T) {
	multi := []byte("中")
	var p input.Parser
	if events := p.Feed(multi[:1]); len(events) != 0 {
		t.Fatalf("half a character decoded as %+v", events)
	}
	events := p.Feed(multi[1:])
	if len(events) != 1 || events[0].(input.Key).Rune != '中' {
		t.Fatalf("got %+v, want the reassembled character", events)
	}
}

func TestABrokenCharacterIsDroppedWithoutTakingTheNextOneWithIt(t *testing.T) {
	var p input.Parser
	// The leading byte of a three-byte character, followed by something that cannot
	// continue it. The pair is already known to be invalid — it does not wait for a
	// flush — and the letter after it still has to arrive.
	events := p.Feed([]byte("中")[:1])
	if len(events) != 0 {
		t.Fatalf("a lone leading byte decoded as %+v", events)
	}
	events = p.Feed([]byte("x"))
	events = append(events, p.Flush()...)
	if len(events) != 1 {
		t.Fatalf("got %+v, want only the letter", events)
	}
	if got := events[0].(input.Key); !got.IsRune('x', 0) {
		t.Fatalf("got %+v, want the letter", got)
	}
}

func TestInvalidUTF8IsDropped(t *testing.T) {
	events := feed("\xffa")
	if len(events) != 1 {
		t.Fatalf("got %+v, want only the letter", events)
	}
	if got := events[0].(input.Key); !got.IsRune('a', 0) {
		t.Fatalf("got %+v", got)
	}
}

func TestMalformedSequenceRecoversAtTheOffendingByte(t *testing.T) {
	// A byte that cannot appear inside a sequence ends it. What follows is ordinary
	// input and must not be swallowed with the malformed prefix.
	events := feed("\x1b[1\x01a")
	if len(events) != 2 {
		t.Fatalf("got %+v, want the control chord and the letter", events)
	}
	if got := events[0].(input.Key); !got.IsRune('a', input.Ctrl) {
		t.Fatalf("first = %+v, want input.Ctrl+A", got)
	}
	if got := events[1].(input.Key); !got.IsRune('a', 0) {
		t.Fatalf("second = %+v, want the letter", got)
	}
}

func TestUnknownSequencesProduceNothing(t *testing.T) {
	for _, in := range []string{"\x1b[9999x", "\x1bOZ", "\x1b[99~"} {
		if events := feed(in); len(events) != 0 {
			t.Errorf("%q produced %+v, want nothing", in, events)
		}
	}
}

func TestKeyMatching(t *testing.T) {
	k := input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl}
	if !k.IsRune('c', input.Ctrl) {
		t.Error("input.Ctrl+C does not match itself")
	}
	// Exactly, not at least: a binding on input.Ctrl+C that also fired for input.Ctrl+input.Shift+C
	// would swallow a keystroke it never claimed.
	withShift := input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl | input.Shift}
	if withShift.IsRune('c', input.Ctrl) {
		t.Error("input.Ctrl+input.Shift+C matched a binding on input.Ctrl+C")
	}
	enter := input.Key{Code: input.Enter}
	if !enter.Is(input.Enter, 0) {
		t.Error("input.Enter does not match itself")
	}
	altEnter := input.Key{Code: input.Enter, Mods: input.Alt}
	if altEnter.Is(input.Enter, 0) {
		t.Error("input.Alt+input.Enter matched a binding on input.Enter")
	}
}

func TestKeyDownCoversRepeats(t *testing.T) {
	// Most handlers want this: holding a key stops working on terminals that report
	// repeats if only a press counts.
	repeat := input.Key{Transition: input.Repeat}
	if !repeat.Down() {
		t.Error("a repeat does not count as going down")
	}
	release := input.Key{Transition: input.Release}
	if release.Down() {
		t.Error("a release counts as going down")
	}
}

func TestKeyString(t *testing.T) {
	for _, tc := range []struct {
		key  input.Key
		want string
	}{
		{input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl}, "ctrl+c"},
		{input.Key{Code: input.Character, Rune: ' '}, "space"},
		{input.Key{Code: input.Character, Rune: ' ', Mods: input.Ctrl}, "ctrl+space"},
		{input.Key{Code: input.Enter}, "enter"},
		{input.Key{Code: input.Backtab}, "shift+tab"},
		{input.Key{Code: input.F5}, "f5"},
		{input.Key{Code: input.Up, Mods: input.Ctrl | input.Shift}, "ctrl+shift+up"},
		{input.Key{Code: input.Character, Rune: 'x', Mods: input.Alt | input.Super}, "alt+super+x"},
	} {
		if got := tc.key.String(); got != tc.want {
			t.Errorf("%+v = %q, want %q", tc.key, got, tc.want)
		}
	}
}

func TestDeviceAttributes(t *testing.T) {
	for _, tc := range []struct {
		in       string
		class    int
		features []int
	}{
		{"\x1b[?62;4;22c", 62, []int{4, 22}},
		{"\x1b[?1;2c", 1, []int{2}},
		{"\x1b[?64c", 64, nil},
		{"\x1b[?6c", 6, nil},
		// A claim that cannot be read is left out, and the rest of the list is
		// still worth having: unlike a key report, a bad number here fires nothing.
		{"\x1b[?62;;4c", 62, []int{4}},
		{"\x1b[?62;99999999999999999999;4c", 62, []int{4}},
	} {
		ev, ok := one(t, tc.in).(input.DeviceAttributes)
		if !ok {
			t.Errorf("%q did not decode as device attributes", tc.in)
			continue
		}
		if ev.Class != tc.class {
			t.Errorf("%q: class = %d, want %d", tc.in, ev.Class, tc.class)
		}
		if !slices.Equal(ev.Features, tc.features) {
			t.Errorf("%q: features = %v, want %v", tc.in, ev.Features, tc.features)
		}
		for _, want := range tc.features {
			if !ev.Has(want) {
				t.Errorf("%q: Has(%d) is false", tc.in, want)
			}
		}
		if ev.Has(0) {
			t.Errorf("%q: Has(0) is true, but zero is never a claim", tc.in)
		}
	}
}

func TestOnlyThePrivateFormIsDeviceAttributes(t *testing.T) {
	// "CSI c" without the marker is a request, not an answer, and a terminal that
	// sent one would be asking this program what it is.
	for _, in := range []string{"\x1b[c", "\x1b[62c", "\x1b[>0;1;0c"} {
		for _, ev := range feed(in) {
			if _, ok := ev.(input.DeviceAttributes); ok {
				t.Errorf("%q decoded as an answer", in)
			}
		}
	}
}

func TestDeviceAttributesWithAClassNobodyCanRead(t *testing.T) {
	// A number beyond any encoding is not a class, and reporting it as a negative
	// one would hand a caller a value the terminal never sent.
	ev, ok := one(t, "\x1b[?99999999999999999999;4c").(input.DeviceAttributes)
	if !ok {
		t.Fatal("the answer did not decode")
	}
	if ev.Class != 0 {
		t.Errorf("class = %d, want 0", ev.Class)
	}
	if !ev.Has(4) {
		t.Error("the claim that could be read was thrown away with the one that could not")
	}
}
