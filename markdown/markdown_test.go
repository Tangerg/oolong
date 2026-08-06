package markdown_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/markdown"
)

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
			c := surface.CellAt(x, y)
			if c.Width() == 0 {
				continue
			}
			if c.Content == "" {
				b.WriteString(" ")
				continue
			}
			b.WriteString(c.Content)
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

func TestWhatCannotBeShownIsSaidRatherThanDropped(t *testing.T) {
	// A link cannot be followed from a row of cells and a picture cannot be drawn in
	// one, so both are written out: the words, and where they point.
	equal(t, render(t, 40, "see [the docs](http://x/y) for more"), []string{
		"see the docs (http://x/y) for more",
	})
	equal(t, render(t, 40, "![a diagram](d.png)"), []string{
		"[a diagram] (d.png)",
	})
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
	want := render(t, 28, source)

	for size := 1; size <= len(source); size++ {
		var stream markdown.Stream
		stream.Look = look()
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
	stream.Look = look()

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

func TestAStreamNeverCutsInsideCode(t *testing.T) {
	// A blank line in a block of code is a blank line in a block of code. Cutting
	// there would publish half a function and render the rest as prose.
	var stream markdown.Stream
	stream.Look = look()
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
