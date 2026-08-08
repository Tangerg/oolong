package text_test

import (
	"strings"
	"testing"

	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/text"
)

// styled writes a line as its spans, each as its text and what it is drawn in, so
// a test can state what colour something came out.
func styled(l text.Line) string {
	var b strings.Builder
	for _, s := range l {
		b.WriteString("[")
		b.WriteString(s.Text)
		b.WriteString(" ")
		b.WriteString(look(s.Style))
		b.WriteString("]")
	}
	return b.String()
}

// attrNames are the six a cell carries, in the order a look reads them out.
var attrNames = []struct {
	bit  grid.Attr
	name string
}{
	{grid.Bold, "bold"},
	{grid.Dim, "dim"},
	{grid.Italic, "italic"},
	{grid.Underline, "underline"},
	{grid.Reverse, "reverse"},
	{grid.Strike, "strike"},
}

func look(s grid.Style) string {
	var b strings.Builder
	for _, attr := range attrNames {
		if s.Attr.Has(attr.bit) {
			b.WriteString(attr.name)
			b.WriteString(" ")
		}
	}
	b.WriteString(colour(s.FG))
	b.WriteString(" on ")
	b.WriteString(colour(s.BG))
	return b.String()
}

func colour(c grid.Color) string {
	if c.Default() {
		return "default"
	}
	rgb := c.RGB()
	return hex(rgb.R) + hex(rgb.G) + hex(rgb.B)
}

func hex(v uint8) string {
	const digits = "0123456789abcdef"
	return string([]byte{digits[v>>4], digits[v&0xf]})
}

func decode(t *testing.T, s string) []string {
	t.Helper()
	out := make([]string, 0, 4)
	for _, l := range text.Decode(s, grid.Style{}) {
		out = append(out, styled(l))
	}
	return out
}

func TestOutputIsReadBackAsTheColoursItWasWrittenIn(t *testing.T) {
	got := decode(t, "plain \x1b[31mred\x1b[0m again")
	want := []string{"[plain  default on default][red 800000 on default][ again default on default]"}
	if len(got) != 1 || got[0] != want[0] {
		t.Fatalf("read %q, want %q", got, want)
	}
}

func TestEveryWayOfNamingAColourReadsAsTheSameColour(t *testing.T) {
	// The three the wire has: one of the sixteen, an index into the palette, and
	// the channels themselves — in both the spelling the standard asks for and the
	// one everything actually writes.
	for _, seq := range []string{
		"\x1b[38;2;255;0;0m", "\x1b[38:2::255:0:0m", "\x1b[38:2:255:0:0m", "\x1b[38;5;9m", "\x1b[91m",
	} {
		lines := text.Decode(seq+"x", grid.Style{})
		if len(lines) != 1 || len(lines[0]) != 1 {
			t.Fatalf("%q came out as %v", seq, lines)
		}
		if got := lines[0][0].Style.FG; got.Default() || got.RGB() != (grid.RGB{R: 255}) {
			t.Errorf("%q is %v, want red", seq, colour(got))
		}
	}
}

func TestAMalformedColourTakesItsOwnParametersWithIt(t *testing.T) {
	// The channels of a colour that could not be read must not be left to be read
	// as attributes: "38;2;999;0;0;1" would otherwise turn the text bold, which is
	// a defect nobody would connect to the colour.
	lines := text.Decode("\x1b[38;2;999;0;0mx", grid.Style{})
	if got := lines[0][0].Style; got.Attr != 0 || !got.FG.Default() {
		t.Fatalf("a malformed colour left %v", look(got))
	}
}

func TestAnAttributeIsTurnedOffByTheParameterThatTurnsItOff(t *testing.T) {
	got := decode(t, "\x1b[1;3mboth\x1b[22mitalic\x1b[mplain")
	want := "[both bold italic default on default][italic italic default on default]" +
		"[plain default on default]"
	if got[0] != want {
		t.Fatalf("read %q, want %q", got[0], want)
	}
}

func TestTheStyleAnInterfaceChoseSurvivesTheOutputRecolouringItself(t *testing.T) {
	// Output drawn inside a themed pane is the pane's body style until something in
	// it says otherwise, and "default foreground" goes back to the pane's rather
	// than to the terminal's.
	base := grid.Style{FG: grid.RGBColor(0x11, 0x22, 0x33)}
	lines := text.Decode("a\x1b[32mb\x1b[39mc", base)
	line := lines[0]
	if len(line) != 3 {
		t.Fatalf("read %d spans, want 3: %s", len(line), styled(line))
	}
	if line[0].Style != base || line[2].Style != base {
		t.Fatalf("the base style did not survive: %s", styled(line))
	}
	if line[1].Style.FG.Default() {
		t.Fatal("the colour the output asked for was dropped")
	}
}

func TestASequenceNothingCanShowIsDroppedRatherThanPrinted(t *testing.T) {
	// A cursor movement, a title, a hyperlink, a charset selection and a sequence
	// that never ended. None of them is text, and none of them may reach a cell.
	got := decode(t, "a\x1b[2Kb\x1b]0;title\x07c\x1b(Bd\re\x1b[")
	if len(got) != 1 || got[0] != "[abcde default on default]" {
		t.Fatalf("read %q", got)
	}
}

func TestALineEndsWhereTheOutputSaidItDoes(t *testing.T) {
	got := decode(t, "one\ntwo\n")
	if len(got) != 2 {
		t.Fatalf("read %d lines, want 2: %q", len(got), got)
	}
	// A trailing newline ends the last line and does not start another: a stream
	// that stopped there has nothing open.
	if got := decode(t, "one\n\nthree"); len(got) != 3 || got[1] != "" {
		t.Fatalf("a blank line came out as %q", got)
	}
}

func TestAStreamIsReadTheSameWayHoweverItArrives(t *testing.T) {
	// The whole point of the decoder over the function: a chunk boundary can fall
	// anywhere, including inside a sequence, and none of it changes the answer.
	whole := "中 \x1b[1mbold\x1b[0m and \x1b[38;5;33m蓝色\x1b[0m\nsecond line\n"
	want := text.Decode(whole, grid.Style{})

	for size := 1; size <= len(whole); size++ {
		var d text.Decoder
		var got []text.Line
		for i := 0; i < len(whole); i += size {
			got = append(got, d.Feed(whole[i:min(i+size, len(whole))])...)
		}
		got = append(got, d.Flush()...)
		if len(got) != len(want) {
			t.Fatalf("in chunks of %d: %d lines, want %d", size, len(got), len(want))
		}
		for i := range got {
			if styled(got[i]) != styled(want[i]) {
				t.Fatalf("in chunks of %d, line %d is %s, want %s",
					size, i, styled(got[i]), styled(want[i]))
			}
		}
	}
}

func TestMalformedUTF8BecomesReplacementText(t *testing.T) {
	lines := text.Decode("before \xff after", grid.Style{})
	if got := lines[0].String(); got != "before \ufffd after" {
		t.Fatalf("malformed UTF-8 decoded as %q", got)
	}
}

func TestTheLineStillArrivingIsThereToBeDrawn(t *testing.T) {
	var d text.Decoder
	if lines := d.Feed("half a li"); len(lines) != 0 {
		t.Fatalf("a line nothing ended was handed over: %q", lines)
	}
	if got := d.Open().String(); got != "half a li" {
		t.Fatalf("the open line is %q", got)
	}
	if lines := d.Feed("ne\n"); len(lines) != 1 || lines[0].String() != "half a line" {
		t.Fatalf("the line finished as %q", lines)
	}
	if got := d.Open(); len(got) != 0 {
		t.Fatalf("something was left open: %q", got.String())
	}

	// Output that stopped without a newline is still output, and Flush is what says
	// nothing more is coming.
	d.Feed("no newline")
	if lines := d.Flush(); len(lines) != 1 || lines[0].String() != "no newline" {
		t.Fatalf("flushed %q", lines)
	}
	if lines := d.Flush(); len(lines) != 0 {
		t.Fatal("flushing twice invented a line")
	}
}

func TestASequenceThatNeverEndsIsNotHeldForEver(t *testing.T) {
	var d text.Decoder
	d.Feed("\x1b]0;" + strings.Repeat("x", 1<<17))
	// Dropped, and the decoder is reading again rather than sitting on a buffer
	// that only grows.
	if lines := d.Feed("after\n"); len(lines) != 1 || lines[0].String() != "after" {
		t.Fatalf("after a runaway sequence the stream read as %q", lines)
	}
}

func TestATabIsLaidOutAndEverythingElseInItsBlockIsNot(t *testing.T) {
	lines := text.Decode("a\tb\x07\x08c", grid.Style{})
	if got := lines[0].String(); got != "a\tbc" {
		t.Fatalf("read %q", got)
	}
	// Which means the width the wrap measures is the width the tab takes.
	if got := lines[0].Width(); got != text.TabStop+2 {
		t.Fatalf("the line measured %d columns", got)
	}
}

func TestWhereTheOutputSaidItsWordsPointIsKept(t *testing.T) {
	// The one thing in the stream that is about the text rather than about the
	// terminal. Without it a command's own hyperlinks are lost between the pipe and
	// the screen, which is the whole point of them.
	lines := text.Decode("see \x1b]8;;http://x/y\x1b\\the docs\x1b]8;;\x1b\\ now", grid.Style{})
	line := lines[0]
	if len(line) != 3 {
		t.Fatalf("read %d spans, want three: %s", len(line), styled(line))
	}
	if line[0].Link != "" || line[2].Link != "" {
		t.Fatalf("text outside the link points at %q and %q", line[0].Link, line[2].Link)
	}
	if line[1].Text != "the docs" || line[1].Link != "http://x/y" {
		t.Fatalf("the linked words are %q pointing at %q", line[1].Text, line[1].Link)
	}

	// And it reaches the cells, which is what a terminal is actually told.
	s := grid.NewSurface(20, 1)
	line.Draw(s.View(), 0, 0)
	if got := cellAt(s, 4).Link; got != "http://x/y" {
		t.Fatalf("the cell under the linked words points at %q", got)
	}
	if got := cellAt(s, 0).Link; got != "" {
		t.Fatalf("a cell outside the link points at %q", got)
	}
}
