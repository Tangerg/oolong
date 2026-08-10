package latex_test

import (
	"testing"

	"github.com/Tangerg/oolong/latex"
)

func TestGlyphsForFollowsTheDrivenTerminalEncoding(t *testing.T) {
	for _, test := range []struct {
		locale string
		plain  bool
	}{
		{"", false},
		{"en_US.UTF-8", false},
		{"C", true},
		{"en_US.ISO-8859-1", true},
	} {
		if got := latex.GlyphsFor(test.locale).Plain; got != test.plain {
			t.Errorf("GlyphsFor(%q).Plain = %t, want %t", test.locale, got, test.plain)
		}
	}
}
