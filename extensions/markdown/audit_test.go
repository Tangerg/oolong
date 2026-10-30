package markdown_test

import (
	"testing"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/text"

	"github.com/Tangerg/oolong/extensions/markdown"
)

func TestMarkdownTextFollowsSemanticNormalization(t *testing.T) {
	for _, test := range [][2]string{
		{`\*literal\* &amp; &#20013;`, "*literal* & 中"},
		{"`one\ntwo`", "one two"},
		{"`&amp;`", "&amp;"},
		{`\&amp;`, "&amp;"},
		{"![a\nb](url)", "[a b]"},
		{"&notit; &#00000065; &#x0000041;", "&notit; &#00000065; &#x0000041;"},
		{"&#65; &#x42; &amp;lt;", "A B &lt;"},
	} {
		got := render(t, 60, test[0])
		if len(got) != 1 || got[0] != test[1] {
			t.Fatalf("%q: %q, want %q", test[0], got, test[1])
		}
	}
}

func TestFenceInfoHasSemanticTextButCodeKeepsItsSource(t *testing.T) {
	for _, info := range []string{"go&#108;ang", `go\+\+`} {
		look := look()
		var gotInfo, gotSource string
		look.SetRenderer(markdown.FencedCode, func(info, source string) (grid.Drawable, error) {
			gotInfo, gotSource = info, source
			return text.NewBlock(text.BlockConfig{}), nil
		})
		if _, err := markdown.Render("```"+info+"\n&amp; \\x\n```", look); err != nil {
			t.Fatal(err)
		}
		want := "golang"
		if info == `go\+\+` {
			want = "go++"
		}
		if gotInfo != want || gotSource != "&amp; \\x" {
			t.Fatalf("info=%q source=%q", gotInfo, gotSource)
		}
	}
}
