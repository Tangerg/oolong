package ansi_test

import (
	"testing"

	"github.com/Tangerg/oolong/core/ansi"
)

// scan reads s to the end and returns what each piece turned out to be, written as
// its kind and its raw bytes, so a test can state the shape of a stream.
func scan(s string) (pieces []string, left string) {
	for s != "" {
		p, n, ok := ansi.Next(s)
		if !ok {
			return pieces, s
		}
		pieces = append(pieces, name(p.Kind)+"("+p.Raw+")")
		s = s[n:]
	}
	return pieces, ""
}

func name(k ansi.Kind) string {
	switch k {
	case ansi.Plain:
		return "text"
	case ansi.Control:
		return "control"
	case ansi.String:
		return "string"
	case ansi.Other:
		return "escape"
	case ansi.Malformed:
		return "malformed"
	default:
		return "?"
	}
}

func equal(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("read %d pieces %q, want %d %q", len(got), got, len(want), want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("piece %d is %q, want %q", i, got[i], want[i])
		}
	}
}

func TestTextRunsUpToTheNextSequenceAndNoFurther(t *testing.T) {
	pieces, left := scan("plain \x1b[1mbold\x1b[m.")
	equal(t, pieces, []string{
		"text(plain )", "control(\x1b[1m)", "text(bold)", "control(\x1b[m)", "text(.)",
	})
	if left != "" {
		t.Fatalf("%q was left over", left)
	}
}

func TestASequenceSplitInTwoIsNotReadAsText(t *testing.T) {
	// The whole of the false case: what is here could still become a sequence, so
	// nothing is decided and nothing is consumed. A reader that answered anyway
	// would put "[3" on the screen and then colour nothing.
	for _, half := range []string{"\x1b", "\x1b[", "\x1b[3", "\x1b]0;ti", "\x1b]0;t\x1b"} {
		if _, n, ok := ansi.Next(half); ok || n != 0 {
			t.Errorf("Next(%q) decided, taking %d bytes", half, n)
		}
	}
}

func TestAStringCommandEndsAtEitherOfItsTwoTerminators(t *testing.T) {
	pieces, _ := scan("\x1b]0;title\x07here\x1b]8;;http://x\x1b\\link")
	equal(t, pieces, []string{
		"string(\x1b]0;title\x07)", "text(here)", "string(\x1b]8;;http://x\x1b\\)", "text(link)",
	})

	// The body is what is between them, and the introducer is what says which
	// command it was.
	p, _, _ := ansi.Next("\x1b]52;c;YWJj\x07")
	if p.Body != "52;c;YWJj" || p.Final != ']' {
		t.Fatalf("read the command as body %q introduced by %q", p.Body, string(p.Final))
	}
}

func TestWhatCannotBeASequenceIsNotReadAsTextEither(t *testing.T) {
	// The introducer would otherwise be printed, and printing "[" because a
	// terminal sent something malformed is how a decoder makes noise nobody can
	// trace.
	pieces, _ := scan("a\x1b[1\x07b")
	equal(t, pieces, []string{"text(a)", "malformed(\x1b[1)", "text(\x07b)"})

	pieces, _ = scan("\x1b\x07x")
	equal(t, pieces, []string{"malformed(\x1b)", "text(\x07x)"})
}

func TestAnEscapeWithIntermediatesIsOnePiece(t *testing.T) {
	// "ESC ( B" selects a character set. Reading two bytes of it would leave the
	// "B" to be printed, which is exactly the noise this is here to prevent.
	pieces, _ := scan("\x1b(Bplain\x1b=")
	equal(t, pieces, []string{"escape(\x1b(B)", "text(plain)", "escape(\x1b=)"})
}

func TestWhatIsLeftOverIsAlwaysAnIncompleteSequenceOrRune(t *testing.T) {
	for _, s := range []string{"tail \x1b[3", "\x1b]0;ti", "text\xe4\xb8"} {
		left := s
		for left != "" {
			_, n, ok := ansi.Next(left)
			if !ok {
				break
			}
			left = left[n:]
		}
		if left == "" {
			t.Fatalf("%q left nothing over", s)
		}
	}
}

func TestAnIncompleteRuneWaitsWithoutHoldingCompleteText(t *testing.T) {
	p, n, ok := ansi.Next("plain \xe4\xb8")
	if !ok || n != len("plain ") || p.Kind != ansi.Plain || p.Raw != "plain " {
		t.Fatalf("first piece = %#v, %d, %v", p, n, ok)
	}
	if _, n, ok := ansi.Next("\xe4\xb8"); ok || n != 0 {
		t.Fatalf("incomplete rune was consumed: n=%d, ok=%v", n, ok)
	}
}

func TestTheParametersAreReadOnceAndTheSameWayForEveryone(t *testing.T) {
	ps := ansi.Parse("<35;10;20")
	if ps.Private != '<' {
		t.Fatalf("the private marker is %q", string(ps.Private))
	}
	if ps.Count() != 3 || ps.At(0) != 35 || ps.At(1) != 10 || ps.At(2) != 20 {
		t.Fatalf("read %v", ps.Groups)
	}

	// An empty field is the protocol's own default, which is zero, and a field that
	// is not a number is -1 so that a decoder can refuse it rather than act on a
	// value invented for it.
	ps = ansi.Parse("1;;x")
	if ps.At(1) != 0 || ps.At(2) != -1 {
		t.Fatalf("an empty field is %d and a malformed one %d", ps.At(1), ps.At(2))
	}
	if n := ansi.Parse("99999999999").At(0); n != -1 {
		t.Fatalf("a number past the limit is %d", n)
	}

	// Subparameters belong to the parameter they were written under.
	if group := ansi.Parse("38:2::1:2:3").Group(0); len(group) != 6 || group[3] != 1 {
		t.Fatalf("read the subparameters as %v", group)
	}
	if !ansi.Parse("").Empty() {
		t.Fatal("a sequence with no parameters carried some")
	}
}

func TestTheBytesOfASequenceAreOneDefinition(t *testing.T) {
	// Everything that reads a sequence asks these, so a disagreement about them is
	// a disagreement between packages.
	if !ansi.Body('3') || !ansi.Body(';') || !ansi.Body('?') || !ansi.Body('(') {
		t.Error("a parameter byte was not one")
	}
	if ansi.Body('m') || ansi.Body(0x07) {
		t.Error("something that ends a sequence was read as part of its body")
	}
	if !ansi.Final('m') || !ansi.Final('~') || ansi.Final(0x1b) {
		t.Error("the final byte is not the one that ends a sequence")
	}
	if !ansi.Body('(') || !ansi.Body(':') {
		t.Error("an intermediate byte or a subparameter separator was not part of a body")
	}
}
