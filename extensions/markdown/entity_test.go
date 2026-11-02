package markdown_test

import (
	"slices"
	"testing"

	"github.com/Tangerg/oolong/extensions/markdown"
	"github.com/Tangerg/oolong/ui/core/grid"
)

func TestEntityWhitespacePreservesWordsAcrossInlineBoundaries(t *testing.T) {
	for _, reference := range []string{"&NewLine;", "&#10;", "&#xA;", "&#13;", "&#xD;", "&#12;", "&#xC;"} {
		for _, test := range []struct{ source, want string }{
			{"a" + reference + "b", "a b"},
			{"a" + reference + "**b**", "a b"},
			{"**a**" + reference + "b", "a b"},
			{"[a" + reference + "**b**](https://example.test)", "a b"},
			{"![a" + reference + "**b**](image.png)", "[a b]"},
		} {
			if got := render(t, 60, test.source); !slices.Equal(got, []string{test.want}) {
				t.Errorf("Render(%q) = %q, want %q", test.source, got, test.want)
			}
		}
	}
}

func TestEntityDecodingPreservesSyntaxAndLinkIdentity(t *testing.T) {
	for _, test := range []struct{ source, want string }{
		{`\&NewLine;`, "&NewLine;"},
		{"`&NewLine;`", "&NewLine;"},
		{"```\na&NewLine;b\n```", "a&NewLine;b"},
		{"&#42;literal&#42;", "*literal*"},
		{"a&#128;b", "ab"},
	} {
		if got := render(t, 60, test.source); !slices.Equal(got, []string{test.want}) {
			t.Errorf("Render(%q) = %q, want %q", test.source, got, test.want)
		}
	}

	var document markdown.Doc
	document.SetBlocks(mustRender(t, "[a&NewLine;**b**](https://example.test/O&#39;Brien)", markdown.Look{}))
	surface := grid.NewSurface(40, 1)
	document.Draw(surface.View())
	if got := cellAt(surface, 0, 0).Link; got != "https://example.test/O'Brien" {
		t.Fatalf("decoded link target = %q", got)
	}
}
