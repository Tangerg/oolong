package grid

import (
	"strings"
	"testing"
)

func TestThePaletteRoundTripsThroughItsOwnIndices(t *testing.T) {
	// Every entry of the cube and the ramp is exactly representable, so asking for
	// the nearest index to a palette colour has to give that colour back. This is
	// the check that catches an off-by-one in either direction of the arithmetic.
	for index := 16; index < 256; index++ {
		rgb := PaletteRGB(uint8(index))
		if got := rgb.Index256(); got != uint8(index) {
			t.Errorf("palette %d is %+v, which maps back to %d", index, rgb, got)
		}
	}
}

func TestThePaletteRegionsAreWhatXtermSays(t *testing.T) {
	for _, tc := range []struct {
		index uint8
		want  RGB
	}{
		{0, RGB{0, 0, 0}},
		{15, RGB{255, 255, 255}},
		{16, RGB{0, 0, 0}},        // first cell of the cube
		{231, RGB{255, 255, 255}}, // last cell of the cube
		{232, RGB{8, 8, 8}},       // first step of the ramp
		{255, RGB{238, 238, 238}}, // last step of the ramp
		{196, RGB{255, 0, 0}},     // pure red in the cube
		{46, RGB{0, 255, 0}},      // pure green
		{21, RGB{0, 0, 255}},      // pure blue
	} {
		if got := PaletteRGB(tc.index); got != tc.want {
			t.Errorf("palette %d = %+v, want %+v", tc.index, got, tc.want)
		}
	}
}

func TestAGreyPrefersTheRampOverTheCube(t *testing.T) {
	// The cube holds six greys and the ramp holds twenty-four. Searching only the
	// cube is what turns every near-grey into a muddy brown, so this is the test
	// that says the ramp is being searched at all.
	for _, v := range []uint8{18, 28, 58, 88, 118, 148, 178, 208, 238} {
		got := RGB{v, v, v}.Index256()
		if got < 232 {
			t.Errorf("grey %d chose palette %d, which is in the cube, not the ramp", v, got)
		}
	}
}

func TestNothingMapsIntoTheSixteenTerminalsRepaint(t *testing.T) {
	// A terminal renders 0–15 however its theme likes, so an index chosen there is
	// a colour nobody can predict. Every truecolor value has to land at 16 or above.
	for _, c := range []RGB{
		{0, 0, 0}, {255, 255, 255}, {128, 0, 0}, {1, 1, 1}, {254, 254, 254},
	} {
		if got := c.Index256(); got < 16 {
			t.Errorf("%+v chose palette %d, which the terminal's theme owns", c, got)
		}
	}
}

func TestTheNearestOfSixteenIsTheObviousOne(t *testing.T) {
	for _, tc := range []struct {
		in   RGB
		want uint8
	}{
		{RGB{0, 0, 0}, 0},
		{RGB{255, 0, 0}, 9},
		{RGB{0, 255, 0}, 10},
		{RGB{0, 0, 255}, 12},
		{RGB{255, 255, 255}, 15},
		{RGB{250, 250, 250}, 15},
		{RGB{130, 0, 0}, 1},
	} {
		if got := tc.in.Index16(); got != tc.want {
			t.Errorf("%+v = ansi %d, want %d", tc.in, got, tc.want)
		}
	}
}

// encode is the SGR one style produces at a depth.
func encode(style Style, depth Depth) string {
	p := painter{depth: depth}
	p.appendStyle(style)
	return string(p.out)
}

func TestTruecolorIsEmittedUnchanged(t *testing.T) {
	got := encode(Style{FG: RGBColor(10, 20, 30)}, TrueColor)
	if !strings.Contains(got, ";38;2;10;20;30") {
		t.Fatalf("sgr = %q, want the 24-bit value", got)
	}
}

func TestAutoIsTruecolorToAnythingThatHasToDrawBeforeItIsTold(t *testing.T) {
	if encode(Style{FG: RGBColor(10, 20, 30)}, Auto) != encode(Style{FG: RGBColor(10, 20, 30)}, TrueColor) {
		t.Fatal("the zero depth drew differently from truecolor")
	}
}

func TestTwoFiftySixUsesTheIndexedForm(t *testing.T) {
	got := encode(Style{FG: RGBColor(255, 0, 0)}, Depth256)
	if !strings.Contains(got, ";38;5;196") {
		t.Fatalf("sgr = %q, want the indexed form of pure red", got)
	}
}

func TestSixteenUsesThePlainAndBrightRanges(t *testing.T) {
	for _, tc := range []struct {
		style Style
		want  string
	}{
		{Style{FG: RGBColor(128, 0, 0)}, ";31"},      // plain red foreground
		{Style{FG: RGBColor(255, 0, 0)}, ";91"},      // bright red foreground
		{Style{BG: RGBColor(0, 128, 0)}, ";42"},      // plain green background
		{Style{BG: RGBColor(255, 255, 255)}, ";107"}, // bright white background
	} {
		got := encode(tc.style, Depth16)
		if !strings.Contains(got, tc.want) {
			t.Errorf("sgr = %q, want %q in it", got, tc.want)
		}
	}
}

func TestNoColorKeepsTheAttributesAndDropsTheColour(t *testing.T) {
	// Bold and underline still carry meaning in a transcript. A colour does not.
	got := encode(Style{FG: RGBColor(255, 0, 0), Attr: Bold | Underline}, NoColor)
	if strings.Contains(got, "38") || strings.Contains(got, "31") {
		t.Fatalf("sgr = %q, want no colour in it", got)
	}
	if !strings.Contains(got, ";1") || !strings.Contains(got, ";4") {
		t.Fatalf("sgr = %q, want bold and underline kept", got)
	}
}

func TestChangingTheDepthRepaintsEverything(t *testing.T) {
	// Every cell the terminal is holding was encoded at the old depth, so a change
	// that only affected the next diff would leave the rest of the screen wrong.
	s := NewScreen(4, 1)
	s.Frame().Text(0, 0, "ab", Style{FG: RGBColor(200, 30, 30)})
	var first buffer
	if err := s.Flush(&first); err != nil {
		t.Fatal(err)
	}
	s.SetDepth(Depth16)
	s.Frame().Text(0, 0, "ab", Style{FG: RGBColor(200, 30, 30)})
	var second buffer
	if err := s.Flush(&second); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(second.String(), ";91") {
		t.Fatalf("after the depth changed the frame wrote %q, want the sixteen-colour form", second.String())
	}
}

func TestSettingTheSameDepthTwiceCostsNothing(t *testing.T) {
	s := NewScreen(4, 1)
	s.SetDepth(Depth256)
	s.Frame().Text(0, 0, "ab", Style{})
	var settled buffer
	if err := s.Flush(&settled); err != nil {
		t.Fatal(err)
	}
	s.SetDepth(Depth256)
	s.Frame().Text(0, 0, "ab", Style{})
	var again buffer
	if err := s.Flush(&again); err != nil {
		t.Fatal(err)
	}
	if again.String() != "" {
		t.Fatalf("an unchanged frame wrote %q, want silence", again.String())
	}
}

type buffer struct{ b strings.Builder }

func (w *buffer) Write(p []byte) (int, error) { return w.b.Write(p) }
func (w *buffer) String() string              { return w.b.String() }
