package markdown_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/text"
	"github.com/Tangerg/oolong/markdown"
)

type cellReader interface {
	CellAt(x, y int) (grid.Cell, bool)
}

func cellAt(r cellReader, x, y int) grid.Cell {
	cell, ok := r.CellAt(x, y)
	if !ok {
		panic("test read outside grid")
	}
	return cell
}

// look is a set of marks a test can read back out of the rows. The styles are left
// alone: what a heading is drawn in is a decision, and what it says is not.
func look() markdown.Look {
	return markdown.Look{Glyphs: markdown.Glyphs{
		Bullet:    "•",
		Bar:       "│",
		Divider:   "─",
		Checked:   "[x]",
		Unchecked: "[ ]",
	}}
}

// rows draws a document and returns what it came to, one string per row, with
// trailing blanks cut so a test can state the shape rather than the padding.
func rows(t *testing.T, width int, blocks []markdown.Block) []string {
	t.Helper()
	// Through SetBlocks rather than the field, because the wrap is remembered and a
	// document given new blocks under it would draw what it used to say.
	doc := &markdown.Doc{}
	doc.SetBlocks(blocks)
	height := doc.Measure(width)
	surface := grid.NewSurface(width, height)
	doc.Draw(surface.View())

	out := make([]string, 0, height)
	for y := range height {
		var b strings.Builder
		for x := range width {
			c := cellAt(surface, x, y)
			if c.Width() == 0 {
				continue
			}
			if c.Content() == "" {
				b.WriteString(" ")
				continue
			}
			b.WriteString(c.Content())
		}
		out = append(out, strings.TrimRight(b.String(), " "))
	}
	return out
}

func equal(t *testing.T, got, want []string) {
	t.Helper()
	if !slices.Equal(got, want) {
		t.Fatalf("drawn:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func render(t *testing.T, width int, source string) []string {
	t.Helper()
	return rows(t, width, markdown.Render(source, look()))
}

func TestProseIsWrappedAtTheWidthItIsDrawnIn(t *testing.T) {
	// The whole reason a block is lines rather than rows: what was written has no
	// width, and the width is whatever the pane turns out to be.
	source := "# Title\n\nA sentence long enough to need two rows of a narrow pane."
	equal(t, render(t, 24, source), []string{
		"Title",
		"",
		"A sentence long enough",
		"to need two rows of a",
		"narrow pane.",
	})
}

func TestDocumentRowsSeparateMeaningfulTextFromItsRenderedOffset(t *testing.T) {
	doc := &markdown.Doc{}
	doc.SetBlocks(markdown.Render("- one two", look()))
	got := doc.Rows(6)
	if len(got) != 2 {
		t.Fatalf("rows = %+v, want two wrapped rows", got)
	}
	if got[0].Text != "one" || got[0].Offset != 2 || got[0].Joined {
		t.Fatalf("first row = %+v", got[0])
	}
	if got[1].Text != "two" || got[1].Offset != 2 || !got[1].Joined || got[1].Gap != " " {
		t.Fatalf("continuation = %+v", got[1])
	}
}

func TestDocumentOwnsItsBlocksAndReturnsSnapshots(t *testing.T) {
	input := markdown.Render("original", look())
	var doc markdown.Doc
	doc.SetBlocks(input)

	input[0] = markdown.Render("changed input", look())[0]
	if got := doc.Rows(40)[0].Text; got != "original" {
		t.Fatalf("document text = %q after input slice mutation", got)
	}
	first := doc.Blocks()
	first[0] = markdown.Render("changed snapshot", look())[0]
	if got := doc.Blocks()[0].Rows(40)[0].Text; got != "original" {
		t.Fatalf("document text = %q after snapshot mutation", got)
	}

	appended := markdown.Render("appended", look())
	doc.Append(appended...)
	appended[0] = markdown.Render("changed append input", look())[0]
	if got := doc.Blocks()[1].Rows(40)[0].Text; got != "appended" {
		t.Fatalf("appended text = %q after caller mutation", got)
	}
}

func TestStreamOpenReturnsAnOwnedSnapshot(t *testing.T) {
	var stream markdown.Stream
	stream.Feed("still being written")
	first := stream.Open()
	if len(first) == 0 || len(first[0].Rows(40)) == 0 {
		t.Fatalf("open rendering = %+v, want text", first)
	}
	want := first[0].Rows(40)[0].Text
	first[0] = markdown.Render("caller mutation", look())[0]
	if got := stream.Open()[0].Rows(40)[0].Text; got != want {
		t.Fatalf("cached open rendering changed to %q, want %q", got, want)
	}
}

func TestStreamLookIsOwnedAndInvalidatesTheOpenRendering(t *testing.T) {
	headings := []grid.Style{{Attr: grid.Bold}}
	var stream markdown.Stream
	stream.SetLook(markdown.Look{Headings: headings})
	stream.Feed("# heading")

	style := func() grid.Style {
		blocks := stream.Open()
		if len(blocks) == 0 || blocks[0].Measure(40) == 0 {
			t.Fatalf("open heading = %+v", blocks)
		}
		surface := grid.NewSurface(40, blocks[0].Measure(40))
		blocks[0].Draw(surface.View())
		return cellAt(surface, 0, 0).Style
	}
	if got := style(); !got.Attr.Has(grid.Bold) {
		t.Fatalf("initial heading style = %+v", got)
	}

	headings[0] = grid.Style{Attr: grid.Italic}
	snapshot := stream.Look()
	snapshot.Headings[0] = grid.Style{Attr: grid.Strike}
	if got := style(); !got.Attr.Has(grid.Bold) {
		t.Fatalf("heading style after external mutation = %+v", got)
	}

	stream.SetLook(markdown.Look{Headings: []grid.Style{{Attr: grid.Italic}}})
	if got := style(); !got.Attr.Has(grid.Italic) || got.Attr.Has(grid.Bold) {
		t.Fatalf("heading style after SetLook = %+v", got)
	}
}

func TestAListIsIndentedPastItsOwnMarks(t *testing.T) {
	// Every row of an item lines up under the first, which is what makes a list of
	// wrapped items readable at all — and what a renderer that put the bullet in the
	// text would lose on the second row.
	source := "- one, which is long enough to wrap\n- two\n  - under two\n\n1. first\n2. second"
	equal(t, render(t, 18, source), []string{
		"• one, which is",
		"  long enough to",
		"  wrap",
		"• two",
		"  • under two",
		"",
		"1. first",
		"2. second",
	})
}

func TestAThematicBreakKeepsTheMarkerOfItsListItem(t *testing.T) {
	// The divider is the item's content, not a reason to discard the item. Keeping
	// the marker is what distinguishes a one-item list containing a rule from a rule
	// at the document root.
	equal(t, render(t, 14, "-   ***"), []string{"• ────────────"})
}

func TestAQuotationKeepsItsBarDownEveryRow(t *testing.T) {
	// Including the rows a wrap produced. A bar beside only the first line is a
	// quotation that stops looking like one exactly where it gets long enough to
	// need the help.
	source := "> quoted text that wraps\n>\n> and carries on"
	equal(t, render(t, 16, source), []string{
		"│ quoted text",
		"│ that wraps",
		"",
		"│ and carries on",
	})
}

func TestNestedQuotationsAdvanceByOneRailPerLevel(t *testing.T) {
	equal(t, render(t, 20, "> > quoted"), []string{"│ │ quoted"})
}

func TestCodeIsLeftExactlyAsItWasWritten(t *testing.T) {
	source := "before\n\n```go\nfunc main() {\n\n\tprintln(\"hi\")\n}\n```\n\nafter"
	equal(t, render(t, 30, source), []string{
		"before",
		"",
		"func main() {",
		"",
		"        println(\"hi\")",
		"}",
		"",
		"after",
	})
}

func TestATableIsPaddedToItsWidestCell(t *testing.T) {
	source := "| name | size |\n| --- | ---: |\n| a | 1 |\n| longer | 200 |"
	equal(t, render(t, 30, source), []string{
		"name   │ size",
		"────── │ ────",
		"a      │    1",
		"longer │  200",
	})
}

func TestATableBecomesLabeledRecordsWhenColumnsStopReading(t *testing.T) {
	source := "| name | description | state |\n" +
		"| --- | --- | --- |\n" +
		"| alpha | fast | ready |\n" +
		"| beta | slow | idle |"
	equal(t, render(t, 17, source), []string{
		"name: alpha",
		"description: fast",
		"state: ready",
		"",
		"name: beta",
		"description: slow",
		"state: idle",
	})
}

func TestATableWrapsCellsInsideAllocatedColumnsWithoutLosingLinks(t *testing.T) {
	source := "| label | target |\n| --- | --- |\n| row | [documentation](https://example.test) |"
	blocks := markdown.Render(source, look())
	doc := &markdown.Doc{}
	doc.SetBlocks(blocks)
	const width = 16
	height := doc.Measure(width)
	surface := grid.NewSurface(width, height)
	doc.Draw(surface.View())

	var linked strings.Builder
	for y := range height {
		for x := range width {
			cell := cellAt(surface, x, y)
			if cell.Link == "https://example.test" && cell.Width() > 0 {
				linked.WriteString(cell.Content())
			}
		}
	}
	if got := linked.String(); got != "documentation" {
		t.Fatalf("linked table text = %q, want documentation", got)
	}
	for i, row := range doc.Rows(width) {
		if got := text.Width(row.Text); got > width {
			t.Fatalf("row %d is %d columns wide at width %d: %q", i, got, width, row.Text)
		}
	}
}

func TestATableLayoutStaysInsideEveryUsableWidth(t *testing.T) {
	source := "| key | description | status |\n" +
		"| :--- | :---: | ---: |\n" +
		"| 名称 | 東京 and a considerably longer value | ✅ |\n" +
		"| emoji | 👩🏽‍💻 handles grapheme clusters | ready |"
	blocks := markdown.Render(source, look())
	doc := &markdown.Doc{}
	doc.SetBlocks(blocks)
	for width := 2; width <= 64; width++ {
		for rowIndex, row := range doc.Rows(width) {
			if end := row.Offset + text.Width(row.Text); end > width {
				t.Fatalf("width %d row %d ends at %d: %q", width, rowIndex, end, row.Text)
			}
		}
		surface := grid.NewSurface(width, doc.Measure(width))
		doc.Draw(surface.View())
	}
}

func TestTheWordsCarryWhereTheyPoint(t *testing.T) {
	// A hyperlink survives the wrap and reaches the cells, which is what a terminal
	// is told and what makes the words themselves clickable.
	blocks := markdown.Render("see [the docs](http://x/y) for more", look())
	doc := &markdown.Doc{}
	doc.SetBlocks(blocks)
	s := grid.NewSurface(40, doc.Measure(40))
	doc.Draw(s.View())
	if got := cellAt(s, 4, 0).Link; got != "http://x/y" {
		t.Fatalf("the cell under the linked words points at %q", got)
	}
	if got := cellAt(s, 0, 0).Link; got != "" {
		t.Fatalf("a cell outside the link points at %q", got)
	}

	// And the address is not written out beside them, because the words are the
	// link. A look with a style for an address gets it written as well, for output
	// going somewhere that cannot show one.
	equal(t, render(t, 40, "see [the docs](http://x/y) for more"), []string{
		"see the docs for more",
	})
	spelled := look()
	spelled.Target = grid.Style{Attr: grid.Dim}
	equal(t, rows(t, 40, markdown.Render("see [the docs](http://x/y) for more", spelled)), []string{
		"see the docs (http://x/y) for more",
	})

	// A picture is what it was called and where it is, and the name points at it.
	equal(t, render(t, 40, "![a diagram](d.png)"), []string{"[a diagram]"})
	// A bare address is its own text, and saying it twice would be noise.
	equal(t, render(t, 40, "at <http://x/y> now"), []string{"at http://x/y now"})
}

func TestTheThingsGithubAddedAreReadToo(t *testing.T) {
	// Tables, tasks, strikethrough and bare addresses are what people actually write,
	// whatever the specification says is core.
	equal(t, render(t, 30, "- [x] done\n- [ ] not"), []string{
		"• [x] done",
		"• [ ] not",
	})
	equal(t, render(t, 30, "~~gone~~ and http://x/y"), []string{"gone and http://x/y"})
}

func TestAStreamComesToTheSameThingHoweverItArrives(t *testing.T) {
	// The property the whole module rests on. A chunk boundary can fall anywhere —
	// inside a word, inside a fence, between the two halves of a list marker — and
	// none of it may change what the reader ends up with.
	source := "# Title\n\nA paragraph that\nis written across lines.\n\n" +
		"- one\n- two\n\n  still two\n\n```go\nx := 1\n\ny := 2\n```\n\n> quoted\n\nlast\n"
	source += "\n| name | state |\n| --- | --- |\n| stream | done |\n"
	want := render(t, 28, source)

	for size := 1; size <= len(source); size++ {
		var stream markdown.Stream
		stream.SetLook(look())
		var blocks []markdown.Block
		for i := 0; i < len(source); i += size {
			blocks = append(blocks, stream.Feed(source[i:min(i+size, len(source))])...)
		}
		blocks = append(blocks, stream.Flush()...)
		if got := rows(t, 28, blocks); !slices.Equal(got, want) {
			t.Fatalf("in chunks of %d:\n%s\nwhole:\n%s",
				size, strings.Join(got, "\n"), strings.Join(want, "\n"))
		}
	}
}

func TestAStreamPublishesWhatIsFinishedAndHoldsWhatIsNot(t *testing.T) {
	var stream markdown.Stream
	stream.SetLook(look())

	// A paragraph is not finished by a blank line alone: a list, and a block of code
	// written with an indent, both carry on across one.
	if got := stream.Feed("- one\n\n"); len(got) != 0 {
		t.Fatalf("a blank line published %d blocks before anything followed it", len(got))
	}
	if got := stream.Feed("  more of one\n\nnext\n"); len(got) == 0 {
		t.Fatal("a line at the left margin published nothing")
	}
	// What is still arriving is rendered as what it says so far, which is what a
	// reader sees while it is being written.
	stream.Feed("## a heading half w")
	if got := rows(t, 30, stream.Open()); len(got) == 0 || got[len(got)-1] != "a heading half w" {
		t.Fatalf("the open part reads as %q", got)
	}
}

func TestAStreamKeepsThePendingCutInsideItsNewTail(t *testing.T) {
	var stream markdown.Stream
	if got := stream.Feed("one\n\ntwo\n\n"); len(got) != 1 || got[0].Rows(20)[0].Text != "one" {
		t.Fatalf("first publication = %+v", got)
	}
	if got := stream.Feed("three\n"); len(got) != 1 || got[0].Rows(20)[0].Text != "two" {
		t.Fatalf("second publication = %+v", got)
	}
}

func TestAStreamNeverCutsInsideCode(t *testing.T) {
	// A blank line in a block of code is a blank line in a block of code. Cutting
	// there would publish half a function and render the rest as prose.
	var stream markdown.Stream
	stream.SetLook(look())
	if got := stream.Feed("```\none\n\ntwo\n\nthree\n"); len(got) != 0 {
		t.Fatalf("%d blocks were published from inside a fence", len(got))
	}
	if got := stream.Feed("```\n\nafter\n"); len(got) == 0 {
		t.Fatal("a closed fence published nothing")
	}
}

func TestABreakIsAsWideAsTheRoomItSeparates(t *testing.T) {
	// Stretched to the width rather than wrapped to it. A rule that wrapped would come
	// out as several rows of dashes, which is the one thing it must not look like.
	equal(t, render(t, 12, "above\n\n---\n\nbelow"), []string{
		"above",
		"",
		"────────────",
		"",
		"below",
	})
}
