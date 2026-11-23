package markdown_test

import (
	"strings"
	"testing"

	"github.com/Tangerg/oolong/extensions/markdown"
)

func TestGlyphsFollowTheTerminalLocale(t *testing.T) {
	if got := markdown.GlyphsFor("en_US.UTF-8"); got != markdown.Unicode() {
		t.Fatalf("a UTF-8 locale chose %+v", got)
	}
	if got := markdown.GlyphsFor(""); got != markdown.Unicode() {
		t.Fatalf("an unspecified locale chose %+v", got)
	}
	ascii := markdown.GlyphsFor("C")
	if ascii != markdown.ASCII() {
		t.Fatalf("a non-UTF locale chose %+v", ascii)
	}
	for _, glyph := range []string{ascii.Bullet, ascii.Bar, ascii.Divider, ascii.Checked, ascii.Unchecked} {
		if strings.ContainsFunc(glyph, func(r rune) bool { return r > 0x7f }) {
			t.Fatalf("ASCII glyphs contain %q", glyph)
		}
	}
}
