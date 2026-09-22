package markdown_test

import (
	"slices"
	"testing"

	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/text"

	"github.com/Tangerg/oolong/markdown"
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

func TestStreamCRLFPublishesStableParagraphs(t *testing.T) {
	var stream markdown.Stream
	blocks := mustFeed(t, &stream, "first\r\n\r\nsecond\r\n")
	if len(blocks) != 1 || blocks[0].Rows(30)[0].Text != "first" {
		t.Fatalf("stable paragraphs: %#v", blocks)
	}
}

func TestStreamDoesNotPublishInsideRawHTML(t *testing.T) {
	for _, source := range []string{"<!--\n\n-->\n\nafter\n", "<?\n\n?>\n\nafter\n", "<![CDATA[\n\n]]>\n\nafter\n", "<script>\n\n</script>\n\nafter\n", "before\n\n<script>\nhidden\n\nnot prose\n</script>\n\nafter\n", "<!-- comment\n\nnot prose\n-->\n\nafter\n"} {
		expected := rows(t, 40, mustRender(t, source, look()))
		for size := 1; size < len(source); size++ {
			stream := &markdown.Stream{}
			stream.SetLook(look())
			var blocks []markdown.Block
			for at := 0; at < len(source); at += size {
				blocks = append(blocks, mustFeed(t, stream, source[at:min(at+size, len(source))])...)
			}
			blocks = append(blocks, mustFlush(t, stream)...)
			got := rows(t, 40, blocks)
			if !slices.Equal(got, expected) {
				t.Fatalf("chunk %d: %q != %q", size, got, expected)
			}
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

func TestAllMarkdownLineEndingsShareStreamingSemantics(t *testing.T) {
	for _, newline := range []string{"\n", "\r", "\r\n"} {
		source := "first" + newline + newline + "second" + newline
		expected := rows(t, 40, mustRender(t, "first\n\nsecond\n", look()))
		for split := 1; split <= len(source); split++ {
			var stream markdown.Stream
			stream.SetLook(look())
			var blocks []markdown.Block
			for at := 0; at < len(source); at += split {
				blocks = append(blocks, mustFeed(t, &stream, source[at:min(at+split, len(source))])...)
			}
			if len(blocks) == 0 {
				t.Fatalf("no stable paragraph for %q split %d", newline, split)
			}
			blocks = append(blocks, mustFlush(t, &stream)...)
			if got := rows(t, 40, blocks); !slices.Equal(got, expected) {
				t.Fatalf("%q split %d: %q != %q", newline, split, got, expected)
			}
		}
	}
}
