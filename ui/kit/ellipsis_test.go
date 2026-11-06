package kit_test

import (
	"testing"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/core/text"
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
			button := kit.NewButton(kit.ButtonConfig{Behavior: base.ButtonConfig{Label: "abcdefgh"}, Ellipsis: tc.ellipsis})
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

func TestParagraphElisionUsesAnIndependentSuffix(t *testing.T) {
	for _, tc := range []struct {
		source, suffix, want string
		width                int
	}{
		{"❤\nnext", "\ufe0f", "\ufe0f", 1},
		{"1\nnext", "\ufe0f\u20e3", "\ufe0f\u20e3", 1},
		{"x🇦\nnext", "🇧🇨", "x🇧🇨", 4},
		{"a\nnext", "…", "a…", 4},
	} {
		t.Run(tc.source, func(t *testing.T) {
			paragraph := kit.NewParagraph(tc.source, grid.Style{})
			paragraph.MaxRows, paragraph.Ellipsis = 1, tc.suffix
			rows := paragraph.Rows(tc.width)
			if len(rows) != 1 || rows[0].Text != tc.want || text.Width(rows[0].Text) > tc.width {
				t.Fatalf("elided rows = %+v, want %q within %d columns", rows, tc.want, tc.width)
			}
		})
	}
}

func TestParagraphElisionCannotGiveItsSuffixASourceLink(t *testing.T) {
	const target = "https://example.test/visible"
	style := grid.Style{Attr: grid.Bold}
	paragraph := kit.NewParagraph("", grid.Style{})
	paragraph.SetText([]text.Line{
		{{Text: "x", Style: style, Link: target}, {Text: "🇦", Style: grid.Style{Attr: grid.Italic}, Link: "https://example.test/removed"}},
		text.Of("next", grid.Style{}),
	})
	paragraph.MaxRows, paragraph.Ellipsis = 1, "🇧🇨"
	surface := grid.NewSurface(4, 1)
	paragraph.Draw(surface.View())
	if got := surface.Rows()[0]; got != "x🇧🇨" {
		t.Fatalf("elided paragraph drew %q, want x🇧🇨", got)
	}
	if cell := cellAt(surface.View(), 0, 0); cell.Link != target || cell.Style != style {
		t.Fatalf("retained cell = %+v, want its original style and link", cell)
	}
	for column := 1; column < 3; column++ {
		if cell := cellAt(surface.View(), column, 0); cell.Link != "" || cell.Style != style {
			t.Fatalf("suffix cell %d = %+v, want the retained style without a link", column, cell)
		}
	}
}
