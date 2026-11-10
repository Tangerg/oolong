package input_test

import (
	"testing"
	"unicode"

	"github.com/Tangerg/oolong/ui/core/input"
)

func TestChordValidationAdmitsOnlyRepresentableShortcutIdentities(t *testing.T) {
	for _, test := range []struct {
		name  string
		chord input.Chord
		valid bool
	}{
		{"enter", input.Chord{Code: input.Enter}, true},
		{"character", input.Ctrl.Rune('x'), true},
		{"space", input.Alt.Rune(' '), true},
		{"keypad", input.Shift.With(input.KeypadBegin), true},
		{"zero width format", input.Chord{Rune: '\u200d'}, true},
		{"no identity", input.Chord{}, false},
		{"functional rune", input.Chord{Code: input.Enter, Rune: 'x'}, false},
		{"control rune", input.Ctrl.Rune('\n'), false},
		{"unknown code", input.Chord{Code: input.KeypadBegin + 1}, false},
		{"unknown modifiers", input.Chord{Rune: 'x', Mods: input.Meta << 1}, false},
		{"invalid scalar", input.Chord{Rune: unicode.MaxRune + 1}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := test.chord.Validate(); (err == nil) != test.valid {
				t.Fatalf("Validate = %v, valid=%v", err, test.valid)
			}
			written, err := test.chord.MarshalText()
			if (err == nil) != test.valid {
				t.Fatalf("MarshalText = %q, %v", written, err)
			}
			if test.valid {
				var restored input.Chord
				if err := restored.UnmarshalText(written); err != nil || restored != test.chord {
					t.Fatalf("configuration round trip = %+v, %v", restored, err)
				}
			}
		})
	}
}

// ParseChord reads only what Chord.String writes. A sign or leading zero in a
// function key, or a control character, named a chord nothing reports.
func TestParseChordRefusesSpellingsStringNeverWrites(t *testing.T) {
	for _, spelling := range []string{"f+1", "ctrl+f+1", "f01", "f-1", "U+001F", "ctrl+U+0000"} {
		if chord, ok := input.ParseChord(spelling); ok {
			t.Errorf("ParseChord(%q) = %+v, want refusal", spelling, chord)
		}
	}
	if chord, ok := input.ParseChord("f12"); !ok || chord.Code != input.F12 {
		t.Fatalf("ParseChord(f12) = %+v, %v", chord, ok)
	}
}

// A Kitty key number in the C0 or C1 range is not a printable identity.
func TestExtendedKeyRefusesControlIdentities(t *testing.T) {
	var p input.Parser
	if events := p.Feed([]byte("\x1b[31u\x1b[133u")); len(events) != 0 {
		t.Fatalf("control key numbers decoded as %+v", events)
	}
}
