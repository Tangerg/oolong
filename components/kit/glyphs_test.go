package kit_test

import (
	"strings"
	"testing"

	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/grid"
)

func env(vars map[string]string) func(string) string {
	return func(name string) string { return vars[name] }
}

// TestGlyphsFollowTheLocale, because there is no way to ask a terminal whether it will
// draw a box character. Outside UTF-8 a multi-byte glyph arrives as bytes the terminal
// draws one at a time, and a panel in mojibake is worse than a panel in dashes.
func TestGlyphsFollowTheLocale(t *testing.T) {
	for _, tc := range []struct {
		name    string
		env     map[string]string
		unicode bool
	}{
		{name: "nothing said", env: map[string]string{}, unicode: true},
		{name: "a UTF-8 locale", env: map[string]string{"LANG": "en_US.UTF-8"}, unicode: true},
		{name: "spelled without the dash", env: map[string]string{"LANG": "en_US.utf8"}, unicode: true},
		{name: "the C locale", env: map[string]string{"LANG": "C"}, unicode: false},
		{name: "POSIX", env: map[string]string{"LC_ALL": "POSIX"}, unicode: false},
		{name: "latin-1", env: map[string]string{"LANG": "en_US.ISO-8859-1"}, unicode: false},
		{name: "a language with no charset", env: map[string]string{"LANG": "en_US"}, unicode: false},

		// The order the C library reads them in.
		{
			name:    "LC_ALL overrides a UTF-8 LANG",
			env:     map[string]string{"LC_ALL": "C", "LANG": "en_US.UTF-8"},
			unicode: false,
		},
		{
			name:    "LC_CTYPE outranks LANG",
			env:     map[string]string{"LC_CTYPE": "en_US.UTF-8", "LANG": "C"},
			unicode: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := kit.GlyphsFor(env(tc.env))
			want := kit.ASCII()
			if tc.unicode {
				want = kit.Unicode()
			}
			if got.Horizontal != want.Horizontal || got.Ellipsis != want.Ellipsis {
				t.Errorf("got %q/%q, want %q/%q",
					got.Horizontal, got.Ellipsis, want.Horizontal, want.Ellipsis)
			}
		})
	}
}

func TestGlyphsWithNothingToAsk(t *testing.T) {
	if got := kit.GlyphsFor(nil); got.Horizontal != kit.Unicode().Horizontal {
		t.Errorf("got %q, want the unicode set", got.Horizontal)
	}
}

// TestEveryGlyphHasAFallback. The set exists so that adding a glyph and forgetting its
// fallback is caught here rather than by somebody running in the C locale.
func TestEveryGlyphHasAFallback(t *testing.T) {
	ascii := kit.ASCII()
	for name, got := range map[string]string{
		"Horizontal": ascii.Horizontal, "Vertical": ascii.Vertical,
		"TopLeft": ascii.TopLeft, "TopRight": ascii.TopRight,
		"BottomLeft": ascii.BottomLeft, "BottomRight": ascii.BottomRight,
		"RoundTopLeft": ascii.RoundTopLeft, "RoundTopRight": ascii.RoundTopRight,
		"RoundBottomLeft": ascii.RoundBottomLeft, "RoundBottomRight": ascii.RoundBottomRight,
		"Ellipsis": ascii.Ellipsis, "Bullet": ascii.Bullet, "Marker": ascii.Marker,
		"ScrollTrack": ascii.ScrollTrack, "ScrollThumb": ascii.ScrollThumb,
	} {
		if got == "" {
			t.Errorf("%s has no fallback", name)
		}
		for _, r := range got {
			if r > 0x7f {
				t.Errorf("%s falls back to %q, which is not ASCII", name, got)
			}
		}
	}
	if len(ascii.Spinner) == 0 {
		t.Error("Spinner has no fallback")
	}
	for _, frame := range ascii.Spinner {
		for _, r := range frame {
			if r > 0x7f {
				t.Errorf("the spinner falls back to %q, which is not ASCII", frame)
			}
		}
	}
}

func TestBordersAreBuiltFromTheGlyphs(t *testing.T) {
	ascii := kit.ASCII()
	for _, b := range []kit.Border{ascii.Rounded(), ascii.Square()} {
		joined := b.Top + b.Left + b.TopLeft + b.BottomRight
		for _, r := range joined {
			if r > 0x7f {
				t.Errorf("an ASCII border is drawn with %q", joined)
			}
		}
	}
	// And a rounded ASCII border is square, because there is no rounded corner in
	// ASCII and pretending otherwise would draw something else.
	if ascii.Rounded() != ascii.Square() {
		t.Error("the two ASCII borders differ, but ASCII has only one corner")
	}
	if kit.Unicode().Rounded() == kit.Unicode().Square() {
		t.Error("the two unicode borders are the same")
	}
}

func TestBoxDrawsWithTheGlyphsItWasGiven(t *testing.T) {
	s := grid.NewSurface(8, 3)
	kit.Box{Glyphs: kit.ASCII(), Border: kit.ASCII().Square()}.Draw(s.View())

	row := rowOf(s.View(), 0, 8)
	if !strings.HasPrefix(row, "+") || !strings.Contains(row, "-") {
		t.Errorf("the top row is %q, want an ASCII border", row)
	}
	for _, r := range row {
		if r > 0x7f {
			t.Errorf("the box drew %q with an ASCII border", row)
			break
		}
	}
}

// rowOf reads one row of a surface back as a string.
func rowOf(v grid.View, y, width int) string {
	var b strings.Builder
	for x := range width {
		if c := cellAt(v, x, y); c.Content != "" {
			b.WriteString(c.Content)
		} else {
			b.WriteByte(' ')
		}
	}
	return b.String()
}

func TestThemeFollowsWhatTheTerminalSaid(t *testing.T) {
	dark := kit.Suited(grid.Ground{BG: grid.RGBColor(0x1a, 0x1b, 0x26)})
	light := kit.Suited(grid.Ground{BG: grid.RGBColor(0xfd, 0xf6, 0xe3)})
	if dark == light {
		t.Fatal("a light and a dark terminal got the same theme")
	}
	if dark != kit.Dark() {
		t.Error("a dark terminal did not get the dark theme")
	}
	if light != kit.Light() {
		t.Error("a light terminal did not get the light theme")
	}

	// A terminal that said nothing gets dark: it is the commoner choice, and light is
	// the one that becomes unreadable when it is guessed wrong.
	if got := kit.Suited(grid.Ground{FG: grid.RGBColor(255, 255, 255)}); got != kit.Dark() {
		t.Error("a terminal that said nothing did not get the dark theme")
	}
}
