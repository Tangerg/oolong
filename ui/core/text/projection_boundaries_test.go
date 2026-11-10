package text_test

import (
	"image"
	"slices"
	"strings"
	"testing"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/text"
)

func assertIndependentProjection(t *testing.T, left, right string) {
	t.Helper()
	first := text.Line{{Text: left, Style: grid.Style{Attr: grid.Bold}, Link: "https://example.test/first"}}
	last := text.Line{{Text: right, Style: grid.Style{Attr: grid.Italic}, Link: "https://example.test/last", BreakBefore: true}}
	line := append(first.Clone(), last...)
	width := first.Width() + last.Width()
	want := grid.NewSurface(max(width, 1), 1)
	first.Draw(want.View(), 0, 0)
	last.Draw(want.View(), first.Width(), 0)
	for name, projected := range map[string]text.Line{
		"source": line, "clone": line.Clone(), "display": line.Display(),
		"wrapped": line.Wrap(0)[0].Line, "truncated": line.Truncate(width, ""), "suffix": line.WithSuffix(width, ""),
	} {
		got := grid.NewSurface(max(width, 1), 1)
		if projected.Width() != width || projected.Draw(got.View(), 0, 0) != width {
			t.Fatalf("%s changed the width of %q + %q from %d to %d", name, left, right, width, projected.Width())
		}
		for column := range max(width, 1) {
			if cellAt(got, column) != cellAt(want, column) {
				t.Fatalf("%s changed cell %d of %q + %q: %+v, want %+v", name, column, left, right, cellAt(got, column), cellAt(want, column))
			}
		}
	}
}

func TestExplicitSpanBoundariesPreserveIndependentAtoms(t *testing.T) {
	for _, pair := range [][2]string{
		{"   ", "🏻X"},
		{"❤", "\ufe0fX"},
		{"👩\u200d", "💻"},
		{"🇦", "🇧🇨"},
		{"ᄀ", "ᅡ"},
		{"\u0600", "X"},
		{"\xc3", "\xa9"},
	} {
		t.Run(pair[0]+pair[1], func(t *testing.T) { assertIndependentProjection(t, pair[0], pair[1]) })
	}
}

func TestProjectedClusterSteppingKeepsBoundariesAndSourceIntervals(t *testing.T) {
	type step struct{ at, next, previous int }
	for _, test := range []struct {
		name     string
		line     text.Line
		clusters []text.Cluster
		steps    []step
	}{
		{
			name: "explicit zero-width boundary",
			line: text.Line{{Text: "❤"}, {Text: "\ufe0fX", BreakBefore: true}},
			clusters: []text.Cluster{
				{Text: "❤", From: 0, To: 3}, {Text: "\ufe0f", From: 3, To: 6}, {Text: "X", From: 6, To: 7},
			},
			steps: []step{{-1, 0, 0}, {0, 3, 0}, {1, 3, 0}, {2, 3, 0}, {3, 6, 0}, {4, 6, 3}, {5, 6, 3}, {6, 7, 3}, {7, 7, 6}, {8, 7, 6}},
		},
		{
			name: "filtered byte gap",
			line: text.Of("A\x00e\u0301X", grid.Style{}),
			clusters: []text.Cluster{
				{Text: "A", From: 0, To: 1}, {Text: "e\u0301", From: 2, To: 5}, {Text: "X", From: 5, To: 6},
			},
			steps: []step{{-1, 0, 0}, {0, 1, 0}, {1, 5, 0}, {2, 5, 0}, {3, 5, 2}, {4, 5, 2}, {5, 6, 2}, {6, 6, 5}, {7, 6, 5}},
		},
		{
			name: "filtered byte inside a joined atom",
			line: text.Of("🇦\x00🇧C", grid.Style{}),
			clusters: []text.Cluster{
				{Text: "🇦🇧", From: 0, To: 9}, {Text: "C", From: 9, To: 10},
			},
			steps: []step{{-1, 0, 0}, {0, 9, 0}, {4, 9, 0}, {5, 9, 0}, {8, 9, 0}, {9, 10, 0}, {10, 10, 9}, {11, 10, 9}},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := slices.Collect(test.line.Clusters()); !slices.Equal(got, test.clusters) {
				t.Fatalf("clusters = %+v, want %+v", got, test.clusters)
			}
			for _, step := range test.steps {
				if next, previous := test.line.NextCluster(step.at), test.line.PrevCluster(step.at); next != step.next || previous != step.previous {
					t.Fatalf("offset %d: next/previous = %d/%d, want %d/%d", step.at, next, previous, step.next, step.previous)
				}
			}
		})
	}
}

func FuzzIndependentSpanBoundariesPreserveAtoms(f *testing.F) {
	for _, pair := range [][2]string{{"   ", "🏻X"}, {"❤", "\ufe0fX"}, {"👩\u200d", "💻"}, {"🇦", "🇧🇨"}, {"\xc3", "\xa9"}} {
		f.Add(pair[0], pair[1])
	}
	f.Fuzz(func(t *testing.T, left, right string) {
		if len(left)+len(right) > 256 || strings.ContainsRune(left+right, '\t') {
			return
		}
		assertIndependentProjection(t, left, right)
	})
}

func TestPartialTabsRetainPositiveAtomColumnsAndProvenance(t *testing.T) {
	for _, test := range []struct {
		source        string
		width, indent int
	}{
		{source: "\t🏻", width: 5, indent: 3},
		{source: "\t🏻X", width: 6, indent: 2},
	} {
		rows := text.Of(test.source, grid.Style{}).Wrap(test.width)
		if len(rows) != 2 || rows[1].From != 0 || rows[1].To != len(test.source) {
			t.Fatalf("partial tab rows = %+v", rows)
		}
		row := rows[1].Line
		surface := grid.NewSurface(test.width, 1)
		row.Draw(surface.View(), 0, 0)
		if cell := cellAt(surface, test.indent); cell.Content() != "🏻" || cell.Width() != 2 {
			t.Fatalf("modifier at column %d = %+v", test.indent, cell)
		}
		if strings.HasSuffix(test.source, "X") && cellAt(surface, test.indent+2).Content() != "X" {
			t.Fatalf("following atom moved: %q", surface.Rows())
		}
		if row.Width() != test.indent+text.Width(strings.TrimPrefix(test.source, "\t")) {
			t.Fatalf("partial-tab row width = %d", row.Width())
		}
	}
}

func TestLayoutCoordinatesAndSnapshotsRetainExplicitBoundaries(t *testing.T) {
	line := text.Line{{Text: "   "}, {Text: "🏻X", BreakBefore: true}}
	projection := text.NewLayout(text.LayoutConfig{Lines: []text.Line{line}, Width: 6, Wrap: text.WrapNone})
	line[1].BreakBefore = false
	row, _ := projection.Row(0)
	if row.Width() != 6 || text.Width(row.Content.String()) != 5 {
		t.Fatalf("line and serialized geometry = %d/%d", row.Width(), text.Width(row.Content.String()))
	}
	for _, test := range []struct{ byte, column int }{{3, 3}, {7, 5}, {8, 6}} {
		if got := projection.Caret(text.Position{Byte: test.byte}); got != image.Pt(test.column, 0) {
			t.Fatalf("caret byte %d = %v", test.byte, got)
		}
	}
	if hit := projection.Hit(image.Pt(4, 0)); hit.Byte != 3 {
		t.Fatalf("hit split the modifier: %+v", hit)
	}
	selected := text.Range{Start: text.Position{Byte: 3}, End: text.Position{Byte: 7}}
	if got := projection.Rects(selected); !slices.Equal(got, []image.Rectangle{image.Rect(3, 0, 5, 1)}) || projection.Text(selected) != "🏻" {
		t.Fatalf("selected geometry/content = %v/%q", got, projection.Text(selected))
	}
	row.Content[1].BreakBefore = false
	if again, _ := projection.Row(0); again.Width() != 6 {
		t.Fatal("returned row mutated the layout's owned boundaries")
	}
}

func TestFillAndPadKeepCompositionBoundaries(t *testing.T) {
	filled := text.Of("🇦", grid.Style{Attr: grid.Bold}).Fill(4)
	surface := grid.NewSurface(4, 1)
	filled.Draw(surface.View(), 0, 0)
	for column := range 4 {
		if cell := cellAt(surface, column); cell.Content() != "🇦" || cell.Width() != 1 {
			t.Fatalf("fill copy %d rejoined its neighbor: %+v", column, cell)
		}
	}
	field := text.Line{{Text: "❤"}, {Text: "\ufe0fX", BreakBefore: true}}.Pad(4, layout.Start)
	composed := append(text.Of("Q", grid.Style{}), field...)
	if composed.Width() != 5 || field.Width() != 4 {
		t.Fatalf("padding lost atom boundaries: %+v", composed)
	}
	clone := text.CloneRows([]text.Row{{Content: field}})
	clone[0].Content[0].Text = "changed"
	if field.String() != "❤\ufe0fX  " {
		t.Fatal("row clone shares span storage")
	}
}
