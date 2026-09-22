package markdown_test

import (
	"slices"
	"testing"

	"github.com/Tangerg/oolong/markdown"
)

func TestMarkdownTextFollowsSemanticNormalization(t *testing.T) {
	for _, test := range [][2]string{
		{`\*literal\* &amp; &#20013;`, "*literal* & 中"},
		{"`one\ntwo`", "one two"},
		{"`&amp;`", "&amp;"},
		{`\&amp;`, "&amp;"},
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
	for _, source := range []string{"before\n\n<script>\nhidden\n\nnot prose\n</script>\n\nafter\n", "<!-- comment\n\nnot prose\n-->\n\nafter\n"} {
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
