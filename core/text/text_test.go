package text_test

import (
	"strings"
	"testing"

	"github.com/Tangerg/oolong/core/text"

	"github.com/Tangerg/oolong/core/grid"
)

type cellReader interface {
	CellAt(x, y int) (grid.Cell, bool)
}

func cellAt(r cellReader, x int) grid.Cell {
	cell, ok := r.CellAt(x, 0)
	if !ok {
		panic("test read outside grid")
	}
	return cell
}

// rows renders wrapped rows as plain strings, one per row, so a test can state
// the shape of the wrap.
func rows(wrapped []text.Wrapped) []string {
	out := make([]string, 0, len(wrapped))
	for _, w := range wrapped {
		out = append(out, w.Line.String())
	}
	return out
}

func equal(t *testing.T, got, want []string) {
	t.Helper()
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("rows = %q, want %q", got, want)
	}
}

func TestWidthCountsColumnsNotBytes(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want int
	}{
		{"", 0},
		{"abc", 3},
		{"中文", 4},
		{"é", 1},
		{"a\tb", 9},     // tab to the next stop at 8, then one column
		{"\t", 8},       // a tab from column zero fills the whole stop
		{"ab\tc", 9},    // two columns, tab to 8, one column
		{"\x1b[31m", 4}, // the escape byte is dropped; what follows is literal text
	} {
		if got := text.Width(tc.in); got != tc.want {
			t.Errorf("text.Width(%q) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

func TestWrapAtWordBoundaries(t *testing.T) {
	line := text.Of("the quick brown fox", grid.Style{})
	equal(t, rows(line.Wrap(10)), []string{"the quick", "brown fox"})
}

func TestWrapConsumesTheSpacesAtABreak(t *testing.T) {
	line := text.Of("aaa   bbb", grid.Style{})
	got := rows(line.Wrap(4))
	equal(t, got, []string{"aaa", "bbb"})
}

func TestWrapMarksContinuationRows(t *testing.T) {
	wrapped := text.Of("one two three", grid.Style{}).Wrap(5)
	if wrapped[0].Joined {
		t.Error("the first row claims to continue something")
	}
	for i, w := range wrapped[1:] {
		if !w.Joined {
			t.Errorf("row %d does not know it is a continuation", i+1)
		}
	}
}

func TestRowSeparatorReconstructsLogicalText(t *testing.T) {
	if got := (text.Row{}).Separator(); got != "\n" {
		t.Fatalf("independent row separator = %q, want newline", got)
	}
	if got := (text.Row{Joined: true, Gap: " "}).Separator(); got != " " {
		t.Fatalf("joined row separator = %q, want its consumed space", got)
	}
}

func TestWrapHardBreaksAWordLongerThanTheWidth(t *testing.T) {
	line := text.Of("abcdefghij", grid.Style{})
	equal(t, rows(line.Wrap(4)), []string{"abcd", "efgh", "ij"})
}

func TestWrapNeverSplitsAWideCluster(t *testing.T) {
	// Three columns hold the letter and one wide cluster exactly.
	equal(t, rows(text.Of("a中文", grid.Style{}).Wrap(3)), []string{"a中", "文"})
	// Two cannot, so the row is left a column short rather than cutting a cluster
	// in half.
	equal(t, rows(text.Of("a中文", grid.Style{}).Wrap(2)), []string{"a", "中", "文"})
	// At width one nothing can hold it, so it gets a row of its own and overflows.
	equal(t, rows(text.Of("中", grid.Style{}).Wrap(1)), []string{"中"})
}

func TestWrapKeepsStylesAcrossBreaks(t *testing.T) {
	red := grid.Style{FG: grid.RGBColor(255, 0, 0)}
	line := text.Line{{Text: "hello ", Style: grid.Style{}}, {Text: "world", Style: red}}
	wrapped := line.Wrap(5)
	equal(t, rows(wrapped), []string{"hello", "world"})
	if got := wrapped[1].Line[0].Style; got != red {
		t.Fatalf("continuation style = %+v, want the span's own", got)
	}
}

func TestWrapMergesNeighbouringSpansOfOneStyle(t *testing.T) {
	line := text.Line{{Text: "ab"}, {Text: "cd"}}
	wrapped := line.Wrap(10)
	if len(wrapped[0].Line) != 1 {
		t.Fatalf("row has %d spans, want them merged into one", len(wrapped[0].Line))
	}
}

func TestWrapWithNoWidthReturnsTheLineWhole(t *testing.T) {
	line := text.Of("some text", grid.Style{})
	equal(t, rows(line.Wrap(0)), []string{"some text"})
}

func TestWrapAnEmptyLineIsOneEmptyRow(t *testing.T) {
	if got := text.Of("", grid.Style{}).Wrap(10); len(got) != 1 || got[0].Line.String() != "" {
		t.Fatalf("wrap of an empty line = %q, want one empty row", rows(got))
	}
}

func TestWrapExpandsTabs(t *testing.T) {
	// A leading tab is eight columns, so only the first word fits in twelve.
	equal(t, rows(text.Of("\tfunc main", grid.Style{}).Wrap(12)), []string{"        func", "main"})
}

func TestTruncate(t *testing.T) {
	for _, tc := range []struct {
		in    string
		width int
		want  string
	}{
		{"hello", 10, "hello"},
		{"hello", 5, "hello"},
		{"hello world", 8, "hello w…"},
		{"hello", 1, "…"},
		{"hello", 0, ""},
		{"中文中文", 5, "中文…"},
	} {
		if got := text.Truncate(tc.in, tc.width, "…"); got != tc.want {
			t.Errorf("text.Truncate(%q, %d) = %q, want %q", tc.in, tc.width, got, tc.want)
		}
	}
}

func TestTruncateLineKeepsStylesAndDressesTheEllipsis(t *testing.T) {
	red := grid.Style{FG: grid.RGBColor(255, 0, 0)}
	line := text.Line{{Text: "keep "}, {Text: "drop this", Style: red}}
	got := line.Truncate(8, "…")

	if text := got.String(); text != "keep dr…" {
		t.Fatalf("truncated = %q", text)
	}
	// The ellipsis belongs to the sentence it ends, so it wears the style of the
	// last text that survived.
	if last := got[len(got)-1]; last.Style != red || !strings.HasSuffix(last.Text, "…") {
		t.Fatalf("last span = %+v, want the ellipsis in the surviving style", last)
	}
}

func TestTruncateLineNeverSplitsAWideCluster(t *testing.T) {
	// Four columns, one for the ellipsis: the second wide cluster cannot half-fit.
	got := text.Of("中文中", grid.Style{}).Truncate(4, "…")
	if text := got.String(); text != "中…" {
		t.Fatalf("truncated = %q, want the wide cluster left out whole", text)
	}
}

func TestDrawPlacesTextOnTheView(t *testing.T) {
	s := grid.NewSurface(12, 1)
	red := grid.Style{FG: grid.RGBColor(255, 0, 0)}
	line := text.Line{{Text: "ab"}, {Text: "cd", Style: red}}

	if got := line.Draw(s.View(), 1, 0); got != 4 {
		t.Fatalf("advance = %d, want 4", got)
	}
	if got := cellAt(s, 1).Content; got != "a" {
		t.Fatalf("cell 1 = %q", got)
	}
	if got := cellAt(s, 3); got.Content != "c" || got.Style != red {
		t.Fatalf("cell 3 = %+v, want the styled span", got)
	}
}

func TestDrawExpandsTabsIntoColumns(t *testing.T) {
	s := grid.NewSurface(16, 1)
	line := text.Of("a\tb", grid.Style{})
	if got := line.Draw(s.View(), 0, 0); got != 9 {
		t.Fatalf("advance = %d, want 9", got)
	}
	if got := cellAt(s, 0).Content; got != "a" {
		t.Fatalf("cell 0 = %q", got)
	}
	// The tab is a gap, not a byte: the next letter lands on the tab stop.
	for x := 1; x < 8; x++ {
		if c := cellAt(s, x); !c.Blank() {
			t.Fatalf("cell %d = %+v, want the tab to have left it blank", x, c)
		}
	}
	if got := cellAt(s, 8).Content; got != "b" {
		t.Fatalf("cell 8 = %q, want the letter on the tab stop", got)
	}
}

func TestControlCharactersAreDroppedRatherThanLaidOut(t *testing.T) {
	// Tool output carries escapes and carriage returns. They have no width to lay
	// out, and a cell holding one would replay it at the terminal.
	line := text.Of("a\x1b[31mb\rc", grid.Style{})
	got := line.Wrap(20)
	if text := got[0].Line.String(); strings.ContainsAny(text, "\x1b\r") {
		t.Fatalf("row = %q, want the control characters gone", text)
	}
}

func TestPrintableKeepsTextAndTabsButNotControlCharacters(t *testing.T) {
	const source = "a\tb\x00\x1b\x7f\u0085c\xff"
	if got := text.Printable(source); got != "a\tbc\ufffd" {
		t.Fatalf("Printable(%q) = %q", source, got)
	}
}

func TestOfEmptyTextIsNoSpans(t *testing.T) {
	if got := text.Of("", grid.Style{}); got != nil {
		t.Fatalf("text.Of(\"\") = %+v, want no spans", got)
	}
}

func TestClustersReportsWhereEachOneStarts(t *testing.T) {
	// A cursor cannot live on a rune boundary: a letter and the accent that
	// modifies it are two runes and one thing on screen.
	var starts []int
	var got []string
	for at, cluster := range text.Clusters("aé中́x") {
		starts = append(starts, at)
		got = append(got, cluster)
	}
	if len(got) != 4 {
		t.Fatalf("stepped %d clusters %q, want 4", len(got), got)
	}
	for i, at := range starts {
		if i > 0 && at <= starts[i-1] {
			t.Fatalf("offsets %v do not advance", starts)
		}
	}
	if got[2] != "中́" {
		t.Fatalf("cluster 2 = %q, want the mark joined to what it modifies", got[2])
	}
}

func TestClustersStopsWhenTheCallerDoes(t *testing.T) {
	seen := 0
	for range text.Clusters("abcdef") {
		seen++
		break
	}
	if seen != 1 {
		t.Fatalf("kept going for %d clusters after the caller stopped", seen)
	}
}

func TestSteppingBetweenClusters(t *testing.T) {
	const s = "a中b"
	for _, tc := range []struct {
		name       string
		at         int
		next, prev int
	}{
		{"from the start", 0, 1, 0},
		{"across a wide cluster", 1, 4, 0},
		{"back over a wide cluster", 4, 5, 1},
		{"at the end", 5, 5, 4},
		{"past the end", 99, 5, 4},
		{"before the start", -1, 0, 0},
	} {
		if got := text.NextCluster(s, tc.at); got != tc.next {
			t.Errorf("%s: NextCluster(%d) = %d, want %d", tc.name, tc.at, got, tc.next)
		}
		if got := text.PrevCluster(s, tc.at); got != tc.prev {
			t.Errorf("%s: PrevCluster(%d) = %d, want %d", tc.name, tc.at, got, tc.prev)
		}
	}
}

func TestColumnsBeforeAnOffset(t *testing.T) {
	if got := text.ColumnOf("a中b", 4); got != 3 {
		t.Fatalf("= %d, want a wide cluster counted as two", got)
	}
	if got := text.ColumnOf("a\tb", 2); got != text.TabStop {
		t.Fatalf("= %d, want the tab expanded to its stop", got)
	}
	if got := text.ColumnOf("abc", 0); got != 0 {
		t.Fatalf("= %d, want nothing before the start", got)
	}
}

func TestTheOffsetAtAColumn(t *testing.T) {
	// How a click, and a cursor moving between lines of different lengths, find
	// where they land.
	const s = "a中b"
	for _, tc := range []struct{ col, want int }{
		{0, 0},
		{1, 1},
		{2, 1}, // inside the wide cluster: back to where it starts
		{3, 4},
		{9, 5}, // past the end
		{-1, 0},
	} {
		if got := text.OffsetAt(s, tc.col); got != tc.want {
			t.Errorf("OffsetAt(%d) = %d, want %d", tc.col, got, tc.want)
		}
	}
	if got := text.OffsetAt("a\tb", text.TabStop); got != 2 {
		t.Fatalf("after a tab = %d, want the offset past it", got)
	}
}

func TestAWrappedRowMeasuresAndDrawsItself(t *testing.T) {
	rows := text.Of("one two three", grid.Style{}).Wrap(7)
	if len(rows) < 2 {
		t.Fatalf("wrapped into %d rows, want more than one", len(rows))
	}
	s := grid.NewSurface(10, len(rows))
	for y, row := range rows {
		if got := row.Draw(s.View(), 0, y); got != row.Width() {
			t.Fatalf("row %d drew %d columns and measures %d", y, got, row.Width())
		}
	}
}

func TestWrappingAWordWiderThanTheRow(t *testing.T) {
	// Broken between clusters, because there is no break opportunity to use, and
	// never through a wide one: half a glyph is worse than a short row.
	rows := text.Of("中中中中", grid.Style{}).Wrap(3)
	if len(rows) != 4 {
		t.Fatalf("wrapped into %d rows, want one pair per row at width 3", len(rows))
	}
	for i, row := range rows {
		if w := row.Width(); w != 2 {
			t.Fatalf("row %d is %d columns, want the wide cluster whole", i, w)
		}
	}
}

func TestAClusterWiderThanTheWholeRowStillGetsOne(t *testing.T) {
	// The alternative is dropping it for ever.
	rows := text.Of("中a", grid.Style{}).Wrap(1)
	if len(rows) != 2 {
		t.Fatalf("wrapped into %d rows, want the wide cluster on one of its own", len(rows))
	}
	if rows[0].Width() != 2 {
		t.Fatalf("the first row is %d columns, want the cluster that overflows it", rows[0].Width())
	}
}

func TestSpacesBeforeAHardBreakAreDropped(t *testing.T) {
	rows := text.Of("ab   cdefghij", grid.Style{}).Wrap(5)
	if len(rows) < 2 {
		t.Fatalf("wrapped into %d rows, want more than one", len(rows))
	}
	if got := rows[1].Line.String(); strings.HasPrefix(got, " ") {
		t.Fatalf("row 1 = %q, want the run of spaces consumed by the break", got)
	}
}

func TestTrailingSpacesAreKeptOnlyWhileTheyFit(t *testing.T) {
	rows := text.Of("ab   ", grid.Style{}).Wrap(4)
	if len(rows) != 1 {
		t.Fatalf("wrapped into %d rows, want one", len(rows))
	}
	if w := rows[0].Width(); w > 4 {
		t.Fatalf("the row is %d columns wide, want no more than 4", w)
	}
}

func TestWrappingNothingStillGivesARow(t *testing.T) {
	// A blank line in a composer is a blank line on screen.
	rows := text.Of("", grid.Style{}).Wrap(10)
	if len(rows) != 1 {
		t.Fatalf("wrapped into %d rows, want the one blank row", len(rows))
	}
	if rows[0].Width() != 0 {
		t.Fatalf("the blank row is %d columns wide", rows[0].Width())
	}
}

func TestTruncatingALineToNothingOrToAnEllipsisThatCannotFit(t *testing.T) {
	line := text.Of("abcdef", grid.Style{})
	if got := line.Truncate(0, "…"); got != nil {
		t.Fatalf("= %v, want nothing at all", got)
	}
	// The ellipsis is itself cut when the space cannot hold it.
	if got := line.Truncate(1, "……").String(); got != "…" {
		t.Fatalf("= %q, want the ellipsis cut to fit", got)
	}
	// A line that fits comes back untouched.
	if got := line.Truncate(99, "…").String(); got != "abcdef" {
		t.Fatalf("= %q, want it left alone", got)
	}
	// Without an ellipsis, the cut is silent.
	if got := line.Truncate(3, "").String(); got != "abc" {
		t.Fatalf("= %q, want a silent cut", got)
	}
}

func TestTruncatingAcrossStyles(t *testing.T) {
	// The ellipsis takes the style of the last text that survived, so it reads as
	// part of the sentence it is ending.
	line := text.Line{
		{Text: "ab", Style: grid.Style{Attr: grid.Bold}},
		{Text: "cdef", Style: grid.Style{Attr: grid.Italic}},
	}
	got := line.Truncate(4, "…")
	last := got[len(got)-1]
	if !strings.HasSuffix(last.Text, "…") {
		t.Fatalf("= %q, want it to end with the ellipsis", got.String())
	}
	if !last.Style.Attr.Has(grid.Italic) {
		t.Fatalf("the ellipsis is %+v, want the style of what it follows", last.Style)
	}
}

func TestTruncatingPlainTextToNothing(t *testing.T) {
	if got := text.Truncate("abc", 0, "…"); got != "" {
		t.Fatalf("= %q, want nothing", got)
	}
	if got := text.Truncate("abcdef", 1, "……"); got != "…" {
		t.Fatalf("= %q, want the ellipsis cut to fit", got)
	}
}

func TestNextClusterFromInsideOne(t *testing.T) {
	// An offset in the middle of a cluster still advances past the whole of it.
	if got := text.NextCluster("中b", 1); got != 3 {
		t.Fatalf("NextCluster from inside 中 = %d, want its end at 3", got)
	}
}

func TestPrevClusterFromInsideOne(t *testing.T) {
	// Scan the complete string rather than a prefix ending in the middle of UTF-8;
	// otherwise the partial rune becomes replacement text and invents a boundary.
	if got := text.PrevCluster("a中文b", 6); got != 4 {
		t.Fatalf("PrevCluster from inside 文 = %d, want its start at 4", got)
	}
	if got := text.PrevCluster("a中文b", 4); got != 1 {
		t.Fatalf("PrevCluster from the boundary after 中 = %d, want 中 at 1", got)
	}
}

// TestWrapRecordsWhereEachRowCameFrom is what makes the wrap invertible. Anything
// found in the text before it was wrapped — a URL, a search match, the end of a
// selection — has to be locatable on the rows afterwards.
func TestWrapRecordsWhereEachRowCameFrom(t *testing.T) {
	const s = "the quick brown fox jumps"
	rows := text.Of(s, grid.Style{}).Wrap(10)
	if len(rows) < 2 {
		t.Fatalf("%q at width 10 produced %d rows", s, len(rows))
	}
	for i, row := range rows {
		drawn := row.Line.String()
		// The range covers the row's text, and the row's text is what the range
		// spans once the break's own spaces are excluded.
		if got := s[row.From:row.To]; got != drawn {
			t.Errorf("row %d covers %q but draws %q", i, got, drawn)
		}
		if i > 0 && rows[i-1].To > row.From {
			t.Errorf("row %d starts at %d, before row %d ended at %d", i, row.From, i-1, rows[i-1].To)
		}
	}
	if last := rows[len(rows)-1]; last.To != len(s) {
		t.Errorf("the last row ends at %d, want %d", last.To, len(s))
	}
}

func TestWrapProvenanceSurvivesAHardBreak(t *testing.T) {
	// A word longer than the row is split between clusters, and both halves still
	// have to say where they came from.
	const s = "https://example.com/a/very/long/path"
	rows := text.Of(s, grid.Style{}).Wrap(12)
	if len(rows) < 3 {
		t.Fatalf("got %d rows, want the word broken across several", len(rows))
	}
	var rejoined strings.Builder
	for i, row := range rows {
		if got := s[row.From:row.To]; got != row.Line.String() {
			t.Errorf("row %d covers %q but draws %q", i, got, row.Line.String())
		}
		rejoined.WriteString(s[row.From:row.To])
	}
	if rejoined.String() != s {
		t.Errorf("the rows rejoin to %q, want %q", rejoined.String(), s)
	}
}

func TestWrapProvenanceWithWideCharacters(t *testing.T) {
	// The counts differ: bytes, columns and clusters are three numbers here.
	const s = "中文 abc 日本語"
	rows := text.Of(s, grid.Style{}).Wrap(6)
	for i, row := range rows {
		if row.From < 0 || row.To > len(s) || row.From > row.To {
			t.Fatalf("row %d has the range [%d,%d) in a string of %d bytes", i, row.From, row.To, len(s))
		}
		if got := s[row.From:row.To]; got != row.Line.String() {
			t.Errorf("row %d covers %q but draws %q", i, got, row.Line.String())
		}
	}
}

func TestWrapProvenanceOfADegenerateWidth(t *testing.T) {
	// A width nothing fits in returns the line whole, and the range has to describe
	// the whole of it rather than nothing.
	const s = "unwrappable"
	rows := text.Of(s, grid.Style{}).Wrap(0)
	if len(rows) != 1 {
		t.Fatalf("got %d rows", len(rows))
	}
	if rows[0].From != 0 || rows[0].To != len(s) {
		t.Errorf("range = [%d,%d), want [0,%d)", rows[0].From, rows[0].To, len(s))
	}
}

func TestStampLinkConvertsBytesToColumns(t *testing.T) {
	// The counts differ, and that is the whole point. A URL after an emoji does not
	// start at the column its byte offset names, and getting it wrong underlines the
	// wrong text — visibly, and only for people whose text is not ASCII.
	const url = "https://a.test"
	for _, tc := range []struct {
		name    string
		s       string
		wantCol int
	}{
		{"plain ASCII", "see " + url + " now", 4},
		// Two clusters of two columns each, then a space.
		{"after a wide character", "中文 " + url, 5},
		{"after an emoji", "🚀 " + url, 3},
		{"after a combining mark", "é " + url, 2},
		{"after a tab", "\t" + url, text.TabStop},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Found rather than written down: a byte offset into UTF-8 spelled out by
			// hand is a test that fails for the wrong reason.
			start := strings.Index(tc.s, url)
			end := start + len(url)
			wantWidth := len(url) // every byte of it is one ASCII column
			surface := grid.NewSurface(40, 1)
			col, width := text.StampLink(surface.View(), 0, 0, tc.s, start, end, url)
			if col != tc.wantCol || width != wantWidth {
				t.Fatalf("stamped column %d width %d, want %d and %d", col, width, tc.wantCol, wantWidth)
			}
			// And the cells it named actually carry it, which is the part a reader
			// of the arithmetic alone cannot check.
			v := surface.View()
			if c := cellAt(v, col); c.Link != url {
				t.Errorf("the first column of the run carries %+v", c)
			}
			if c := cellAt(v, col+width-1); c.Link != url {
				t.Errorf("the last column of the run carries %+v", c)
			}
			if c := cellAt(v, col-1); c.Link != "" {
				t.Errorf("the column before the run carries %q", c.Link)
			}
			if c := cellAt(v, col+width); c.Link != "" {
				t.Errorf("the column after the run carries %q", c.Link)
			}
		})
	}
}

func TestStampLinkRefusesARangeItCannotUse(t *testing.T) {
	surface := grid.NewSurface(20, 1)
	for _, tc := range []struct{ start, end int }{
		{-1, 5},
		{0, 100},
		{5, 5},
		{6, 3},
	} {
		col, width := text.StampLink(surface.View(), 0, 0, "some text here", tc.start, tc.end, "https://a.test")
		if col != 0 || width != 0 {
			t.Errorf("[%d,%d) stamped column %d width %d", tc.start, tc.end, col, width)
		}
	}
}

func TestClassOf(t *testing.T) {
	for _, tc := range []struct {
		cluster string
		want    text.Class
	}{
		{" ", text.Space},
		{"\t", text.Space},
		{"a", text.Word},
		{"Z", text.Word},
		{"7", text.Word},
		{"_", text.Word},
		{"é", text.Word},
		{"中", text.Han},
		{"あ", text.Kana},
		{"ア", text.Kana},
		{"한", text.Hangul},
		{".", text.Punct},
		{"-", text.Punct},
		{"", text.Space},
	} {
		if got := text.ClassOf(tc.cluster); got != tc.want {
			t.Errorf("%q = %v, want %v", tc.cluster, got, tc.want)
		}
	}
}

// TestWordAtTakesTheRunOfOneScript is the refinement that matters for text written
// without spaces. Taking the alphabetic rule would swallow the Latin beside a CJK
// phrase; taking one character would select less than anybody meant.
func TestWordAtTakesTheRunOfOneScript(t *testing.T) {
	for _, tc := range []struct {
		name string
		s    string
		at   int
		want string
	}{
		{name: "a word", s: "the quick brown", at: 5, want: "quick"},
		{name: "the first word", s: "the quick", at: 0, want: "the"},
		{name: "the last word", s: "the quick", at: 8, want: "quick"},
		{name: "an underscore is part of one", s: "a snake_case name", at: 5, want: "snake_case"},
		{name: "digits are too", s: "go1.26 here", at: 1, want: "go1"},
		{name: "punctuation is its own", s: "a.b", at: 1, want: "."},
		{name: "a CJK phrase", s: "见 中文词组 here", at: len("见 中"), want: "中文词组"},
		{name: "CJK does not swallow the latin beside it", s: "中文abc", at: 0, want: "中文"},
		{name: "and the latin does not swallow the CJK", s: "中文abc", at: len("中文"), want: "abc"},
		{name: "kana is its own script", s: "中文ひらがな", at: len("中文"), want: "ひらがな"},
		{name: "hangul too", s: "한글text", at: 0, want: "한글"},
		{name: "a combining mark stays with its letter", s: "café here", at: 3, want: "café"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			start, end, ok := text.WordAt(tc.s, tc.at)
			if !ok {
				t.Fatalf("found no word at %d in %q", tc.at, tc.s)
			}
			if got := tc.s[start:end]; got != tc.want {
				t.Errorf("found %q, want %q", got, tc.want)
			}
		})
	}
}

func TestWordAtFindsNothingWorthSelecting(t *testing.T) {
	// Whitespace is not a word, so a double-click in the margin selects nothing rather
	// than selecting the gap.
	for _, tc := range []struct {
		s  string
		at int
	}{
		{"the quick", 3},
		{"  ", 0},
		{"", 0},
		{"abc", -1},
		{"abc", 3},
		{"abc", 99},
	} {
		if start, end, ok := text.WordAt(tc.s, tc.at); ok {
			t.Errorf("%q at %d found %q", tc.s, tc.at, tc.s[start:end])
		}
	}
}

func TestWordAtLandsOnAClusterBoundary(t *testing.T) {
	// An offset inside a multi-byte character belongs to that character, so a caller
	// working in columns cannot get half of one.
	const s = "中文词组"
	start, end, ok := text.WordAt(s, 1)
	if !ok {
		t.Fatal("found no word")
	}
	if got := s[start:end]; got != s {
		t.Errorf("found %q, want the whole run", got)
	}
}
