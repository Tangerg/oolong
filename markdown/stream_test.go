package markdown_test

import (
	"slices"
	"testing"

	"github.com/Tangerg/oolong/markdown"
)

func TestAFenceAfterABlankLineCommitsWhatCameBefore(t *testing.T) {
	// A fence at the left margin after a blank line begins something new, the same as
	// any other line does. Clearing the pending cut without taking it meant a
	// document that opened a block of code after a paragraph never committed anything
	// again: every feed re-parsed the whole answer from the top.
	var stream markdown.Stream
	got := mustFeed(t, &stream, "a paragraph\n\n```go\nfmt.Println()\n")
	if len(got) != 1 {
		t.Fatalf("committed %d blocks, want the paragraph", len(got))
	}
	if text := got[0].Rows(40)[0].Text; text != "a paragraph" {
		t.Fatalf("committed %q", text)
	}
}

func TestAStreamNeverCutsInsideABlockOfCode(t *testing.T) {
	// A fence inside a list item is indented past what the top-level syntax allows,
	// and this scan does not know how deep the item is. Reading it as prose would let
	// a blank line inside the code produce a cut, and a cut inside a block of code
	// splits a literal across two parses.
	var stream markdown.Stream
	source := "- step\n\n      ```\n      one\n\n      two\n      ```\n"
	if got := mustFeed(t, &stream, source); len(got) != 0 {
		t.Fatalf("committed %d blocks while a block of code was still open", len(got))
	}
}

// TestWhereAStreamCutsDoesNotChangeWhatTheDocumentSays is the contract the cut rule
// exists to serve, stated as one property instead of as a list of the documents
// somebody thought of.
//
// A stream publishes a prefix and re-parses the rest, so a cut is only ever allowed
// where parsing the two halves says what parsing the whole says. Where a raw block
// begins and ends is the parser's own answer — how deep a container indents its
// contents, whether a run of backticks inside a block closes it, whether an HTML
// block is still open — and a second lexer with its own opinion agrees with it right
// up until the document where it does not.
func TestWhereAStreamCutsDoesNotChangeWhatTheDocumentSays(t *testing.T) {
	for name, source := range map[string]string{
		"a fence opened on a list item's own line": "- ```\n  first\n\n  *second*\n  ```\n\nafter\n",
		"backticks indented inside a code block":   "```\nfirst\n    ```\n\n*second*\n```\n\nafter\n",
		"a fence with an info string of its own":   "```{.python caption=\"x\"}\none\n\ntwo\n```\n\nafter\n",
		"a fence inside a block quote":             "> ```\n> one\n>\n> two\n> ```\n\nafter\n",
		"a fence inside an indented list item":     "- step\n\n      ```\n      one\n\n      two\n      ```\n\nafter\n",
		"a fence inside an HTML block":             "<div>\n\n```\none\n\ntwo\n```\n\n</div>\n\nafter\n",
		"display mathematics with a blank line":    "$$\na\n\nb\n$$\n\nafter\n",
		"a paragraph and a block of code":          "a paragraph\n\n```go\nfmt.Println()\n```\n\nafter\n",
		"nothing unusual at all":                   "# title\n\none\n\n- a\n- b\n\ntwo\n",
	} {
		t.Run(name, func(t *testing.T) {
			want := rows(t, 40, mustRender(t, source, look()))
			for _, size := range []int{1, 7, len(source)} {
				var stream markdown.Stream
				stream.SetLook(look())
				var blocks []markdown.Block
				for at := 0; at < len(source); at += size {
					blocks = append(blocks, mustFeed(t, &stream, source[at:min(at+size, len(source))])...)
				}
				blocks = append(blocks, mustFlush(t, &stream)...)
				if got := rows(t, 40, blocks); !slices.Equal(got, want) {
					t.Errorf("in %d-byte chunks the document reads\n%q\nwant\n%q", size, got, want)
				}
			}
		})
	}
}
