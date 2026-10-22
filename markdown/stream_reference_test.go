package markdown_test

import (
	"strconv"
	"testing"

	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/markdown"
)

func TestStreamRetainsEarlierReferenceDefinitionsAcrossPublication(t *testing.T) {
	source := "[ref]: https://example.test/first \"title\"\n\n[first][ref]\n\n[second][ref]\n\nend\n"
	want := mustRender(t, source, markdown.Look{})
	for _, chunk := range []int{1, 3, 17, len(source)} {
		t.Run(strconv.Itoa(chunk), func(t *testing.T) {
			var stream markdown.Stream
			var blocks []markdown.Block
			for at := 0; at < len(source); at += chunk {
				blocks = append(blocks, mustFeed(t, &stream, source[at:min(at+chunk, len(source))])...)
				_ = mustOpen(t, &stream)
			}
			blocks = append(blocks, mustFlush(t, &stream)...)
			equal(t, rows(t, 60, blocks), rows(t, 60, want))
			for _, block := range blocks[:2] {
				surface := grid.NewSurface(60, block.HeightForWidth(60))
				block.Draw(surface.View())
				if got := cellAt(surface, 0, 0).Link; got != "https://example.test/first" {
					t.Fatalf("published reference destination = %q", got)
				}
			}
		})
	}
}

func TestStreamReferenceDefinitionsKeepTheFirstPublishedMeaning(t *testing.T) {
	var stream markdown.Stream
	mustFeed(t, &stream, "[A B]: /first\n\nfirst\n")
	mustFeed(t, &stream, "\n[a  b]: /second\n\n[link][a b]\n")
	stream.SetLook(markdown.Look{Link: grid.Style{Attr: grid.Underline}})
	blocks := mustFlush(t, &stream)
	if len(blocks) != 1 {
		t.Fatalf("remaining blocks = %d", len(blocks))
	}
	surface := grid.NewSurface(20, 1)
	blocks[0].Draw(surface.View())
	cell := cellAt(surface, 0, 0)
	if cell.Link != "/first" || !cell.Style.Attr.Has(grid.Underline) {
		t.Fatalf("later definition or look replaced the first definition: %+v", cell)
	}
}

func TestOpenDoesNotPublishAnIncompleteReferenceDefinition(t *testing.T) {
	var stream markdown.Stream
	mustFeed(t, &stream, "[ref]: /part")
	mustOpen(t, &stream)
	mustFeed(t, &stream, "ial\n\n[link][ref]\n")
	blocks := mustFlush(t, &stream)
	if len(blocks) != 1 {
		t.Fatalf("remaining blocks = %d", len(blocks))
	}
	surface := grid.NewSurface(20, 1)
	blocks[0].Draw(surface.View())
	if got := cellAt(surface, 0, 0).Link; got != "/partial" {
		t.Fatalf("Open committed an unfinished reference: %q", got)
	}
}

func TestEndingAStreamReleasesItsReferenceDefinitions(t *testing.T) {
	for _, end := range []string{"Flush", "Reset"} {
		t.Run(end, func(t *testing.T) {
			var stream markdown.Stream
			mustFeed(t, &stream, "[ref]: /previous\n\ntext\n")
			if end == "Flush" {
				mustFlush(t, &stream)
			} else {
				stream.Reset()
			}
			mustFeed(t, &stream, "[link][ref]\n")
			equal(t, rows(t, 40, mustFlush(t, &stream)), []string{"[link][ref]"})
		})
	}
}
