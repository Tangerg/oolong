package markdown_test

import (
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
