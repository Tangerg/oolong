package markdown

import (
	"strings"
	"testing"

	"github.com/Tangerg/oolong/core/grid"
)

func drawn(t *testing.T, width int, blocks []Block) []string {
	t.Helper()
	doc := &Doc{}
	doc.SetBlocks(blocks)
	height := doc.HeightForWidth(width)
	surface := grid.NewSurface(width, height)
	doc.Draw(surface.View())

	out := make([]string, height)
	for y := range height {
		var row strings.Builder
		for x := range width {
			cell, ok := surface.CellAt(x, y)
			if !ok || cell.Width() == 0 {
				continue
			}
			if cell.Content() == "" {
				row.WriteString(" ")
				continue
			}
			row.WriteString(cell.Content())
		}
		out[y] = strings.TrimRight(row.String(), " ")
	}
	return out
}

func streamed(t *testing.T, source string, chunk int) []Block {
	t.Helper()
	var stream Stream
	var blocks []Block
	for at := 0; at < len(source); at += chunk {
		blocks = append(blocks, mustFeed(t, &stream, source[at:min(at+chunk, len(source))])...)
		// Asked for on every chunk because that is what a caller drawing a live answer
		// does, and because the cache behind it must not change what is published.
		_ = mustOpen(t, &stream)
	}
	return append(blocks, mustFlush(t, &stream)...)
}

// FuzzStreamSaysTheSameThingHoweverItArrives is the contract that makes streaming
// possible at all.
//
// A stream publishes a prefix and never looks at it again, so a cut is only allowed
// where splitting the source there does not change what either half says. Where the
// reads fell is not something the writer of a document chose, and it is the one thing
// that must not reach the reader.
//
// It is not compared against rendering the whole document, because that is a
// different claim and a weaker one: a stream deliberately publishes a reference link
// before its address arrives — see [Stream] — and the price of that is stated rather
// than hidden.
func FuzzStreamSaysTheSameThingHoweverItArrives(f *testing.F) {
	for _, source := range []string{
		"plain text",
		"$$\n\\frac{a}{b}\n$$\n",
		"```math\nx^2\n```\n",
		"$$\nnot closed",
		"> - nested\n>\n> content\n",
		"```a`b\n\nafter\n\nmore\n",
		"<!--\n```\n-->\n\nafter\n",
		"- ```go\n  x\n  ```\n\nafter\n",
		"one\n\ntwo\n\nthree\n",
		"| a | b |\n| - | - |\n| 1 | 2 |\n\nafter\n",
	} {
		f.Add(source)
	}
	f.Fuzz(func(t *testing.T, source string) {
		if len(source) > 8<<10 {
			t.Skip()
		}
		whole := drawn(t, 40, streamed(t, source, max(len(source), 1)))
		for _, chunk := range []int{1, 3, 17} {
			got := drawn(t, 40, streamed(t, source, chunk))
			if len(got) != len(whole) {
				t.Fatalf("in %d-byte chunks the document is %d rows and whole it is %d:\n%q\n%q",
					chunk, len(got), len(whole), got, whole)
			}
			for i := range got {
				if got[i] != whole[i] {
					t.Fatalf("in %d-byte chunks row %d is %q and whole it is %q",
						chunk, i, got[i], whole[i])
				}
			}
		}
	})
}
