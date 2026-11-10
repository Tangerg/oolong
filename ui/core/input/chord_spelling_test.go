package input_test

import (
	"testing"

	"github.com/Tangerg/oolong/ui/core/input"
)

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
