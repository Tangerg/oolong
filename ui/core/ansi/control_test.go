package ansi_test

import (
	"testing"

	"github.com/Tangerg/oolong/ui/core/ansi"
)

func TestStringIntroducersAreEscapeAndTheirFamilyByte(t *testing.T) {
	for introducer, family := range map[string]ansi.StringFamily{
		ansi.OSCIntroducer: ansi.OSC,
		ansi.DCSIntroducer: ansi.DCS,
		ansi.APCIntroducer: ansi.APC,
	} {
		if introducer != string([]byte{ansi.Escape, byte(family)}) {
			t.Errorf("introducer %q does not open family %q", introducer, string(family))
		}
	}
}

func TestAppendCSISeparatesParametersAndEndsWithItsFunction(t *testing.T) {
	for _, test := range []struct {
		function ansi.Function
		params   []int
		want     string
	}{
		{ansi.CUP, []int{3, 7}, "\x1b[3;7H"},
		{ansi.DECSTBM, nil, "\x1b[r"},
		{ansi.CUU, []int{1}, "\x1b[1A"},
	} {
		if got := string(ansi.AppendCSI(nil, test.function, test.params...)); got != test.want {
			t.Errorf("AppendCSI(%q, %v) = %q, want %q", string(test.function), test.params, got, test.want)
		}
	}
}

func TestModesSetAndResetUnderTheDECPrivateMarker(t *testing.T) {
	for _, test := range []struct {
		mode       ansi.Mode
		set, reset string
	}{
		{ansi.ModeCursorVisible, "\x1b[?25h", "\x1b[?25l"},
		{ansi.ModeAltScreen, "\x1b[?1049h", "\x1b[?1049l"},
		{ansi.ModeSynchronizedOutput, "\x1b[?2026h", "\x1b[?2026l"},
	} {
		if got := test.mode.Set(); got != test.set {
			t.Errorf("mode %d set = %q, want %q", test.mode, got, test.set)
		}
		if got := test.mode.Reset(); got != test.reset {
			t.Errorf("mode %d reset = %q, want %q", test.mode, got, test.reset)
		}
	}
}
