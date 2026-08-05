package input_test

import (
	"strings"
	"testing"

	"github.com/Tangerg/oolong/core/input"
)

// osc decodes a string that must produce exactly one operating system command.
func osc(t *testing.T, s string) input.OSC {
	t.Helper()
	ev, ok := one(t, s).(input.OSC)
	if !ok {
		t.Fatalf("decoding %q produced %T, want input.OSC", s, ev)
	}
	return ev
}

func TestOSCReplies(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   string
		want input.OSC
	}{
		{
			// What a terminal answers a background-colour query with, and the
			// reason any of this exists.
			name: "background colour ended with BEL",
			in:   "\x1b]11;rgb:1a1a/1b1b/2626\x07",
			want: input.OSC{Command: 11, Params: "rgb:1a1a/1b1b/2626"},
		},
		{
			name: "background colour ended with ST",
			in:   "\x1b]11;rgb:ffff/ffff/ffff\x1b\\",
			want: input.OSC{Command: 11, Params: "rgb:ffff/ffff/ffff"},
		},
		{
			name: "a clipboard read, whose payload is base64 and holds a semicolon",
			in:   "\x1b]52;c;aGVsbG8=\x07",
			want: input.OSC{Command: 52, Params: "c;aGVsbG8="},
		},
		{
			name: "the longest command number in use",
			in:   "\x1b]1337;File=name:x\x07",
			want: input.OSC{Command: 1337, Params: "File=name:x"},
		},
		{
			name: "no parameters at all",
			in:   "\x1b]11\x07",
			want: input.OSC{Command: 11, Params: ""},
		},
		{
			name: "no parameters, ended with ST",
			in:   "\x1b]11\x1b\\",
			want: input.OSC{Command: 11, Params: ""},
		},
		{
			name: "an empty parameter section is not the same as none",
			in:   "\x1b]52;\x07",
			want: input.OSC{Command: 52, Params: ""},
		},
		{
			name: "a leading zero is still a number",
			in:   "\x1b]007;title\x07",
			want: input.OSC{Command: 7, Params: "title"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := osc(t, tc.in); got != tc.want {
				t.Errorf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

// TestOSCIntroducerIsAlsoAKeystroke pins the ambiguity the decoder exists to
// resolve. A terminal sends the same two bytes for Alt+] as for the start of an
// operating system command, so what follows has to decide.
func TestOSCIntroducerIsAlsoAKeystroke(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   string
		want input.Key
	}{
		{
			name: "Alt+] alone",
			in:   "\x1b]",
			want: input.Key{Code: input.Character, Rune: ']', Mods: input.Alt},
		},
		{
			name: "Alt+] then a letter",
			in:   "\x1b]a",
			want: input.Key{Code: input.Character, Rune: ']', Mods: input.Alt},
		},
		{
			name: "Alt+] then a number that never gets its separator",
			in:   "\x1b]11a",
			want: input.Key{Code: input.Character, Rune: ']', Mods: input.Alt},
		},
		{
			name: "a number too long to be a command",
			in:   "\x1b]123456;x\x07",
			want: input.Key{Code: input.Character, Rune: ']', Mods: input.Alt},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var p input.Parser
			events := p.Feed([]byte(tc.in))
			// "\x1b]" alone is ambiguous until time says otherwise, which is the
			// same wait a lone escape gets.
			if len(events) == 0 {
				events = p.Flush()
			}
			if len(events) == 0 {
				t.Fatalf("decoding %q produced nothing", tc.in)
			}
			got, ok := events[0].(input.Key)
			if !ok {
				t.Fatalf("decoding %q produced %T first, want a key", tc.in, events[0])
			}
			if got != tc.want {
				t.Errorf("first event = %+v, want %+v", got, tc.want)
			}
		})
	}
}

// TestOSCSplitAcrossReads is the property the whole parser is built for: where a
// read happens to land must not change what the bytes mean.
func TestOSCSplitAcrossReads(t *testing.T) {
	const reply = "\x1b]11;rgb:1a1a/1b1b/2626\x07"
	want := input.OSC{Command: 11, Params: "rgb:1a1a/1b1b/2626"}

	for split := 1; split < len(reply); split++ {
		var p input.Parser
		events := append(p.Feed([]byte(reply[:split])), p.Feed([]byte(reply[split:]))...)
		if len(events) != 1 {
			t.Fatalf("split at %d produced %d events, want 1: %+v", split, len(events), events)
		}
		got, ok := events[0].(input.OSC)
		if !ok {
			t.Fatalf("split at %d produced %T, want input.OSC", split, events[0])
		}
		if got != want {
			t.Errorf("split at %d = %+v, want %+v", split, got, want)
		}
	}
}

func TestOSCOneByteAtATime(t *testing.T) {
	const reply = "\x1b]52;c;aGk=\x1b\\"
	var p input.Parser
	var events []input.Event
	for i := range len(reply) {
		events = append(events, p.Feed([]byte{reply[i]})...)
	}
	if len(events) != 1 {
		t.Fatalf("got %d events, want 1: %+v", len(events), events)
	}
	want := input.OSC{Command: 52, Params: "c;aGk="}
	if got := events[0].(input.OSC); got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

// TestOSCAbandonedByAnEscape covers a terminal that stops mid-answer. The cost
// has to be the command, not every keystroke after it.
func TestOSCAbandonedByAnEscape(t *testing.T) {
	events := feed("\x1b]11;rgb:1a\x1b[A")
	if len(events) != 1 {
		t.Fatalf("got %d events, want 1: %+v", len(events), events)
	}
	want := input.Key{Code: input.Up}
	if got, ok := events[0].(input.Key); !ok || got != want {
		t.Errorf("got %+v, want %+v", events[0], want)
	}
}

func TestOSCFollowedByOrdinaryInput(t *testing.T) {
	events := feed("\x1b]11;rgb:0/0/0\x07ab")
	if len(events) != 3 {
		t.Fatalf("got %d events, want 3: %+v", len(events), events)
	}
	if _, ok := events[0].(input.OSC); !ok {
		t.Fatalf("first event is %T, want input.OSC", events[0])
	}
	for i, want := range []rune{'a', 'b'} {
		got, ok := events[i+1].(input.Key)
		if !ok || got.Rune != want {
			t.Errorf("event %d = %+v, want the character %q", i+1, events[i+1], want)
		}
	}
}

// TestOSCParamsOverrunIsDroppedWhole is the bound. Without it a terminal that
// opens a command and never ends it grows memory without limit; without dropping
// the rest, the overrun arrives as a flood of keystrokes instead.
func TestOSCParamsOverrunIsDroppedWhole(t *testing.T) {
	var p input.Parser
	events := p.Feed([]byte("\x1b]52;" + strings.Repeat("A", 1<<20+16)))
	if len(events) != 0 {
		t.Fatalf("the overrun produced %d events, want none: %+v", len(events), events)
	}
	if !p.Pending() {
		t.Fatal("Pending is false while the rest of a runaway command is still being dropped")
	}
	// The terminator ends the drop, and what follows is ordinary input again.
	events = p.Feed([]byte("\x07xy"))
	if len(events) != 2 {
		t.Fatalf("after the terminator: got %d events, want 2: %+v", len(events), events)
	}
	if got := events[0].(input.Key); got.Rune != 'x' {
		t.Errorf("first event after the drop = %+v, want the character 'x'", got)
	}
}

func TestOSCOverrunEndedByST(t *testing.T) {
	var p input.Parser
	p.Feed([]byte("\x1b]52;" + strings.Repeat("A", 1<<20+16)))
	events := p.Feed([]byte("\x1b\\z"))
	if len(events) != 1 {
		t.Fatalf("got %d events, want 1: %+v", len(events), events)
	}
	if got := events[0].(input.Key); got.Rune != 'z' {
		t.Errorf("got %+v, want the character 'z'", got)
	}
}

// TestOSCOverrunAbandonedByAnEscape leaves the escape for whatever comes next
// rather than swallowing it as a terminator it never was.
func TestOSCOverrunAbandonedByAnEscape(t *testing.T) {
	var p input.Parser
	p.Feed([]byte("\x1b]52;" + strings.Repeat("A", 1<<20+16)))
	events := p.Feed([]byte("\x1b[B"))
	if len(events) != 1 {
		t.Fatalf("got %d events, want 1: %+v", len(events), events)
	}
	want := input.Key{Code: input.Down}
	if got := events[0].(input.Key); got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

// TestOSCStillArrivingSurvivesAFlush is the distinction between the two waits. A
// command already given up on has to end when input goes quiet; one still
// arriving must not, or a large answer split across a pause is corrupted.
func TestOSCStillArrivingSurvivesAFlush(t *testing.T) {
	var p input.Parser
	if events := p.Feed([]byte("\x1b]52;c;aGVs")); len(events) != 0 {
		t.Fatalf("a partial command produced %d events: %+v", len(events), events)
	}
	if events := p.Flush(); len(events) != 0 {
		t.Fatalf("flushing a partial command produced %d events: %+v", len(events), events)
	}
	events := p.Feed([]byte("bG8=\x07"))
	if len(events) != 1 {
		t.Fatalf("got %d events, want 1: %+v", len(events), events)
	}
	want := input.OSC{Command: 52, Params: "c;aGVsbG8="}
	if got := events[0].(input.OSC); got != want {
		t.Errorf("got %+v, want %+v — the pause corrupted the answer", got, want)
	}
}

func TestOSCParamsAreValidUTF8(t *testing.T) {
	got := osc(t, "\x1b]52;c;\xff\xfe\x07")
	if !strings.ContainsRune(got.Params, '�') {
		t.Errorf("params %q kept invalid UTF-8 instead of replacing it", got.Params)
	}
}

// TestOSCInsideAPasteIsText because a paste is bytes the terminal was handed by
// something else, and nothing in it is addressed to this program.
func TestOSCInsideAPasteIsText(t *testing.T) {
	events := feed("\x1b[200~\x1b]11;x\x07\x1b[201~")
	if len(events) != 1 {
		t.Fatalf("got %d events, want 1: %+v", len(events), events)
	}
	paste, ok := events[0].(input.Paste)
	if !ok {
		t.Fatalf("got %T, want input.Paste", events[0])
	}
	if want := "\x1b]11;x\x07"; paste.Text != want {
		t.Errorf("pasted text = %q, want %q", paste.Text, want)
	}
}

// TestOSCOverrunWaitsForTheRestOfAnST covers the one byte of ambiguity in the
// terminator: an escape at the end of a chunk is not yet known to be ST.
func TestOSCOverrunWaitsForTheRestOfAnST(t *testing.T) {
	var p input.Parser
	p.Feed([]byte("\x1b]52;" + strings.Repeat("A", 1<<20+16)))
	if events := p.Feed([]byte("\x1b")); len(events) != 0 {
		t.Fatalf("a lone escape mid-drop produced %+v", events)
	}
	if !p.Pending() {
		t.Fatal("Pending is false while the drop is still waiting on a terminator")
	}
	events := p.Feed([]byte("\\q"))
	if len(events) != 1 {
		t.Fatalf("got %d events, want 1: %+v", len(events), events)
	}
	if got := events[0].(input.Key); got.Rune != 'q' {
		t.Errorf("got %+v, want the character 'q'", got)
	}
}

// TestOSCOverrunEndsWhenInputGoesQuiet is the state that has to end somewhere. A
// command already given up on must not eat the next keystroke that happens to be
// a parameter byte.
func TestOSCOverrunEndsWhenInputGoesQuiet(t *testing.T) {
	var p input.Parser
	p.Feed([]byte("\x1b]52;" + strings.Repeat("A", 1<<20+16)))
	p.Flush()
	if p.Pending() {
		t.Fatal("the drop survived a flush")
	}
	events := p.Feed([]byte("z"))
	if len(events) != 1 {
		t.Fatalf("got %d events, want 1: %+v", len(events), events)
	}
	if got := events[0].(input.Key); got.Rune != 'z' {
		t.Errorf("got %+v, want the character 'z'", got)
	}
}
