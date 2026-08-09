package kit_test

import (
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/diff"
	"github.com/Tangerg/oolong/core/grid"
)

func changed(before, after string) []diff.Hunk {
	return diff.Between(strings.Split(before, "\n"), strings.Split(after, "\n")).Hunks(1)
}

func TestADiffMarksEachLineWithWhatHappenedToIt(t *testing.T) {
	d := kit.NewDiff(kit.Dark(), kit.Glyphs{}, changed("a\nb\nc", "a\nB\nc"))
	rows := paint(10, d.Measure(10), func(v grid.View) { d.Draw(v) })
	want := []string{" a........", "-b........", "+B........", " c........"}
	for i, row := range want {
		if rows[i] != row {
			t.Fatalf("row %d = %q, want %q", i, rows[i], row)
		}
	}
}

func TestADiffColoursTheWholeRowAndNotJustTheText(t *testing.T) {
	// A background that stopped where the text did would leave a diff looking like
	// ragged bunting, and the eye reads the block of colour before it reads the mark.
	theme := kit.Dark()
	d := kit.NewDiff(theme, kit.Glyphs{}, changed("a", "b"))
	s := grid.NewSurface(10, d.Measure(10))
	d.Draw(s.View())

	removed := cellAt(s, 9, 0)
	if removed.Style.BG != theme.Removed.BG {
		t.Fatalf("the far end of a removed line is %+v, want the removed background", removed)
	}
	added := cellAt(s, 9, 1)
	if added.Style.BG != theme.Added.BG {
		t.Fatalf("the far end of an added line is %+v, want the added background", added)
	}
}

func TestADiffPutsBothLineNumbersDownTheLeft(t *testing.T) {
	// A reader looking for what to open needs the number in the text that still
	// exists; one reading the change needs both. A line that is only in one text has a
	// blank where the other number would be.
	d := kit.NewDiff(kit.Dark(), kit.Glyphs{}, changed("a\nb\nc", "a\nB\nc"))
	d.ShowNumbers(true)
	rows := paint(12, d.Measure(12), func(v grid.View) { d.Draw(v) })
	want := []string{"1 1  a.....", "2   -b.....", "  2 +B.....", "3 3  c....."}
	for i, row := range want {
		if !strings.HasPrefix(rows[i], strings.TrimRight(row, ".")) {
			t.Fatalf("row %d = %q, want it to start %q", i, rows[i], strings.TrimRight(row, "."))
		}
	}
}

func TestADiffWrapsLongLinesWithoutDiscardingTheirTail(t *testing.T) {
	d := kit.NewDiff(kit.Dark(), kit.Unicode(), []diff.Hunk{{
		Lines: diff.Script{{Kind: diff.Added, Text: "alpha beta gamma"}},
	}})
	if got := d.Measure(8); got != 3 {
		t.Fatalf("Measure(8) = %d, want three wrapped rows", got)
	}
	equalRows(t, paint(8, d.Measure(8), d.Draw), []string{
		"+alpha..",
		"│beta...",
		"│gamma..",
	})
}

func TestADiffLineNumbersYieldBeforeTheyCrushTheContent(t *testing.T) {
	d := kit.NewDiff(kit.Dark(), kit.Unicode(), []diff.Hunk{{Lines: diff.Script{{
		Kind: diff.Added, Text: "value", Old: 123, New: 456,
	}}}})
	d.ShowNumbers(true)
	if got := d.Measure(10); got != 1 {
		t.Fatalf("Measure(10) = %d, want the content kept on one row", got)
	}
	rows := paint(10, d.Measure(10), d.Draw)
	if strings.Contains(rows[0], "123") || strings.Contains(rows[0], "456") {
		t.Fatalf("narrow diff retained line numbers: %q", rows[0])
	}
	if !strings.HasPrefix(rows[0], "+value") {
		t.Fatalf("narrow diff = %q, want the complete changed text", rows[0])
	}
}

func TestADiffSaysWhereItLeftLinesOut(t *testing.T) {
	before := strings.Repeat("same\n", 10) + "old" + strings.Repeat("\nsame", 10)
	after := strings.Repeat("same\n", 10) + "new" + strings.Repeat("\nsame", 10)
	one := changed(before, after)
	if got := len(one); got != 1 {
		t.Fatalf("%d hunks, want the unchanged middle left out", got)
	}

	two := kit.NewDiff(kit.Dark(), kit.Unicode(),
		changed("a\nx\nb\nc\nd\ne\nf\ny\ng", "a\nX\nb\nc\nd\ne\nf\nY\ng"))
	rows := paint(12, two.Measure(12), func(v grid.View) { two.Draw(v) })
	if got := len(two.Hunks()); got != 2 {
		t.Fatalf("%d hunks, want two", got)
	}
	joined := strings.Join(rows, "\n")
	if !strings.Contains(joined, kit.Unicode().Ellipsis) {
		t.Fatalf("drawn:\n%s\nwant a break saying lines were left out", joined)
	}
}

func TestADiffIgnoresAZeroWidthGapGlyph(t *testing.T) {
	hunks := append(changed("a", "b"), changed("c", "d")...)
	drawn := make(chan struct{})
	go func() {
		defer close(drawn)
		diffView := kit.NewDiff(kit.Theme{}, kit.Glyphs{Ellipsis: "\u0301"}, hunks)
		paint(8, diffView.Measure(8), diffView.Draw)
	}()
	select {
	case <-drawn:
	case <-time.After(time.Second):
		t.Fatal("zero-width gap glyph made drawing stop advancing")
	}
}

func TestADiffTallerThanItsPaneScrolls(t *testing.T) {
	// Which is the whole point of the window being a separate thing: a diff knows how
	// tall it is and nothing else, and something that shows a window onto anything at
	// all does the rest.
	d := kit.NewDiff(kit.Dark(), kit.Glyphs{}, changed("a\nb\nc\nd", "A\nB\nC\nD"))
	window := headless.NewViewport(headless.Static{Of: d})
	paintWidget(10, 3, window)
	window.Scroll().ToBottom()
	rows := paintWidget(10, 3, window)
	if !strings.Contains(strings.Join(rows, "\n"), "D") {
		t.Fatalf("drawn:\n%s\nwant the end of the change", strings.Join(rows, "\n"))
	}
}

func TestADiffOwnsItsHunksAndInvalidatesLayoutThroughSetHunks(t *testing.T) {
	source := []diff.Hunk{{Lines: diff.Script{{Kind: diff.Added, Text: "before"}}}}
	d := kit.NewDiff(kit.Dark(), kit.Unicode(), source)
	if got := d.Measure(12); got != 1 {
		t.Fatalf("Measure(12) = %d, want one row", got)
	}

	source[0].Lines[0].Text = "changed elsewhere"
	snapshot := d.Hunks()
	snapshot[0].Lines[0].Text = "changed snapshot"
	if got := strings.Join(paint(12, 1, d.Draw), ""); !strings.Contains(got, "before") {
		t.Fatalf("draw after caller and snapshot mutation = %q, want owned text", got)
	}

	d.SetHunks([]diff.Hunk{{Lines: diff.Script{{Kind: diff.Added, Text: "after wrapping"}}}})
	if got := d.Measure(8); got != 2 {
		t.Fatalf("Measure(8) after SetHunks = %d, want invalidated two-row layout", got)
	}
}
