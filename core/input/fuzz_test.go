package input_test

import (
	"testing"
	"unicode/utf8"

	"github.com/Tangerg/oolong/core/input"
)

// The parser is the one place in this repository that reads bytes somebody else
// wrote. A terminal sends what it likes, a multiplexer rewrites it on the way, and
// a pasted payload can carry anything at all — so "it did not crash on the
// sequences we thought of" is not the property worth asserting. These are.

// seeds are the shapes worth starting from: every kind of sequence the parser
// claims to understand, plus the ones that are deliberately malformed.
var seeds = []string{
	"",
	"a",
	"\x1b",
	"\x1b\x1b",
	"\x1b[A",
	"\x1b[1;5A",
	"\x1bOP",
	"\x1b[200~pasted\x1b[201~",
	"\x1b[<0;10;20M",
	"\x1b[<0;10;20m",
	"\x1b[I",
	"\x1b[O",
	"\x1b[97;2u",
	"\x1b[97;2;98u",
	"\x1b[1;1;1;1;1;1;1;1;1;1m",
	"\x1b[999999999999999999999;1u",
	"\x1b[-1;-1u",
	"\x1b[;;;;u",
	"\x1b]11;rgb:1a1a/1b1b/2626\x07",
	"\x1b]11;rgb:0/0/0\x1b\\",
	"\x1b]52;c;aGVsbG8=\x07",
	"\x1b]11\x07",
	"\x1b]",
	"\x1b]a",
	"\x1b]999999;x\x07",
	"\x1b]52;unterminated",
	"\x1b]52;abandoned\x1bmore",
	"\xff\xfe\xfd",
	"\xe4\xb8",
	"中文",
	"\x00\x01\x02\x7f",
}

func FuzzParserNeverPanics(f *testing.F) {
	for _, seed := range seeds {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(_ *testing.T, in []byte) {
		var p input.Parser
		p.Feed(in)
		p.Flush()
	})
}

func FuzzParserEmitsOnlyValidRunes(f *testing.F) {
	// A Key carries a rune, and a rune that is not one is a rune something further
	// up will write into a cell and a terminal will be handed. The parser reads
	// numbers out of a sequence and turns them into runes, so this is the boundary
	// where an arbitrary integer stops being arbitrary.
	for _, seed := range seeds {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, in []byte) {
		var p input.Parser
		for _, ev := range append(p.Feed(in), p.Flush()...) {
			key, ok := ev.(input.Key)
			if !ok {
				continue
			}
			if key.Rune != 0 && !utf8.ValidRune(key.Rune) {
				t.Fatalf("key %#v carries rune %d, which is not a valid rune", key, key.Rune)
			}
			if key.Text != "" && !utf8.ValidString(key.Text) {
				t.Fatalf("key %#v carries text %q, which is not valid UTF-8", key, key.Text)
			}
		}
	})
}

func FuzzParserEmitsOnlyValidText(f *testing.F) {
	// Every string this parser hands out came from bytes somebody else wrote, and
	// every one of them ends up in a document or on a screen. Invalid UTF-8
	// reaching a cell is a glyph nobody can measure.
	for _, seed := range seeds {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, in []byte) {
		var p input.Parser
		for _, ev := range append(p.Feed(in), p.Flush()...) {
			switch ev := ev.(type) {
			case input.Paste:
				if !utf8.ValidString(ev.Text) {
					t.Fatalf("paste carries %q, which is not valid UTF-8", ev.Text)
				}
			case input.OSC:
				if !utf8.ValidString(ev.Params) {
					t.Fatalf("command %d carries %q, which is not valid UTF-8", ev.Command, ev.Params)
				}
			case input.Key:
				if ev.Text != "" && !utf8.ValidString(ev.Text) {
					t.Fatalf("key carries %q, which is not valid UTF-8", ev.Text)
				}
			}
		}
	})
}

// FuzzOSCCommandIsNeverNegative because a command number is fed straight into a
// switch by whoever asked the question, and a negative one means the digits
// overflowed on the way in.
func FuzzOSCCommandIsNeverNegative(f *testing.F) {
	for _, seed := range seeds {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, in []byte) {
		var p input.Parser
		for _, ev := range append(p.Feed(in), p.Flush()...) {
			if osc, ok := ev.(input.OSC); ok && osc.Command < 0 {
				t.Fatalf("command number %d is negative", osc.Command)
			}
		}
	})
}

func FuzzParserDoesNotCareHowBytesArrive(f *testing.F) {
	// The property the whole design rests on: a terminal splits its output wherever
	// the read happened to land, so feeding the same bytes one at a time has to
	// produce the same events as feeding them at once. A parser that only worked on
	// whole sequences would drop keys under load and look like a flaky terminal.
	for _, seed := range seeds {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, in []byte) {
		var whole input.Parser
		atOnce := append(whole.Feed(in), whole.Flush()...)

		var piecemeal input.Parser
		var byByte []input.Event
		for i := range in {
			byByte = append(byByte, piecemeal.Feed(in[i:i+1])...)
		}
		byByte = append(byByte, piecemeal.Flush()...)

		if len(atOnce) != len(byByte) {
			t.Fatalf("%q fed whole gave %d events and fed byte by byte gave %d:\n whole: %#v\n split: %#v",
				in, len(atOnce), len(byByte), atOnce, byByte)
		}
		for i := range atOnce {
			if atOnce[i] != byByte[i] {
				t.Fatalf("%q event %d: whole gave %#v, byte by byte gave %#v",
					in, i, atOnce[i], byByte[i])
			}
		}
	})
}

func FuzzFlushResolvesEverythingItCan(f *testing.F) {
	// Flush declares the wait over, so a second one has nothing left to say. A
	// parser that needed two would hold a byte until the next keystroke, which is
	// the shape of every "the first Escape does nothing" bug.
	//
	// Stated as idempotence rather than as "nothing is pending", because one thing
	// legitimately survives a flush: a paste that has been opened and not closed is
	// incomplete rather than ambiguous, and cutting it short would corrupt the text
	// it is carrying.
	for _, seed := range seeds {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, in []byte) {
		var p input.Parser
		p.Feed(in)
		p.Flush()
		if again := p.Flush(); len(again) != 0 {
			t.Fatalf("%q: a second flush produced %#v, so the first did not resolve", in, again)
		}
	})
}
