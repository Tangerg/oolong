package kit_test

import (
	"testing"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/kit"
)

func TestTruncationUsesTheSuppliedGlyphCapability(t *testing.T) {
	for _, tc := range []struct {
		name, ellipsis, paragraph, button string
	}{
		{name: "ASCII", ellipsis: kit.GlyphsFor("C").Ellipsis, paragraph: "a...", button: "[ a... ]"},
		{name: "Unicode", ellipsis: kit.GlyphsFor("C.UTF-8").Ellipsis, paragraph: "abc…", button: "[ abc… ]"},
		{name: "silent", paragraph: "abcd", button: "[ abcd ]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			paragraph := kit.NewParagraph("abcdefgh", grid.Style{})
			paragraph.MaxRows, paragraph.Ellipsis = 1, tc.ellipsis
			if got := paragraph.Rows(4); len(got) != 1 || got[0].Text != tc.paragraph {
				t.Fatalf("paragraph rows=%+v, want %q", got, tc.paragraph)
			}
			button := kit.NewButton(kit.ButtonConfig{Label: "abcdefgh", Ellipsis: tc.ellipsis})
			root, err := scene.NewRoot(scene.RootConfig{Element: button.Element()})
			if err != nil {
				t.Fatal(err)
			}
			defer root.Close()
			surface := grid.NewSurface(8, 1)
			frame, err := root.Prepare(surface.View())
			if err != nil {
				t.Fatal(err)
			}
			frame.Commit(1)
			if got := transcriptRow(surface.View(), 0); got != tc.button {
				t.Fatalf("button=%q, want %q", got, tc.button)
			}
			if got := button.Element().Measure(layout.Across, 1); got != 12 {
				t.Fatalf("button intrinsic width=%d, want complete label and brackets", got)
			}
		})
	}
}

func TestParagraphRewrapsWhenItsEllipsisChanges(t *testing.T) {
	paragraph := kit.NewParagraph("abcdefgh", grid.Style{})
	paragraph.MaxRows = 1
	for _, tc := range []struct{ ellipsis, want string }{
		{ellipsis: kit.Unicode().Ellipsis, want: "abc…"},
		{ellipsis: kit.ASCII().Ellipsis, want: "a..."},
		{want: "abcd"},
	} {
		paragraph.Ellipsis = tc.ellipsis
		if got := paragraph.Rows(4)[0].Text; got != tc.want {
			t.Fatalf("changed ellipsis produced %q, want %q", got, tc.want)
		}
	}
	paragraph.Ellipsis = kit.ASCII().Ellipsis
	if got := paragraph.Rows(2)[0].Text; got != ".." {
		t.Fatalf("marker larger than viewport=%q, want a fitting marker", got)
	}
}
