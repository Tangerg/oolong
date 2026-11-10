package latex_test

import (
	"fmt"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/Tangerg/oolong/extensions/latex"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/text"
)

func TestLiteralGraphemesKeepTheirGeometryInsideFractions(t *testing.T) {
	for _, test := range []struct {
		cluster string
		width   int
	}{
		{cluster: "👩‍💻", width: 2},
		{cluster: "👩🏽‍💻", width: 2},
		{cluster: "🇨🇳", width: 2},
		{cluster: "e\u0301", width: 1},
		{cluster: "α\u0301", width: 1},
	} {
		t.Run(test.cluster, func(t *testing.T) {
			source := `\frac{` + test.cluster + `}{x}`
			formula := latex.Render(source, latex.Look{})
			if err := formula.Err(); err != nil {
				t.Fatal(err)
			}
			want := []string{" " + test.cluster, strings.Repeat("─", test.width+2), " x"}
			if got := rendered(t, source); !slices.Equal(got, want) || formula.Width() != test.width+2 {
				t.Fatalf("fraction = %q width=%d; want %q width=%d", got, formula.Width(), want, test.width+2)
			}
		})
	}
}

func TestUnbracedScriptsConsumeOneCompleteGrapheme(t *testing.T) {
	for _, cluster := range []string{"👩‍💻", "👩🏽‍💻", "🇨🇳", "e\u0301", "α\u0301"} {
		for _, template := range []string{`x^%sa`, `x_%sa`, `x^%s_2`, `\frac{x}{y}^%sa`} {
			source := fmt.Sprintf(template, cluster)
			grouped := fmt.Sprintf(template, "{"+cluster+"}")
			if got, want := rendered(t, source), rendered(t, grouped); !slices.Equal(got, want) {
				t.Errorf("Render(%q) = %q; grouped atom = %q", source, got, want)
			}
		}
	}
	if got, want := rendered(t, "x^🇨🇳🇯"), rendered(t, "x^{🇨🇳}🇯"); !slices.Equal(got, want) {
		t.Fatalf("regional-indicator script = %q; want %q", got, want)
	}
}

func TestGraphemeJoiningCannotConsumeFormulaSyntax(t *testing.T) {
	for _, source := range []string{"{\u0600}", "{x^\u0600}", "\\sqrt[x^\u0600]{x}"} {
		if err := latex.Render(source, latex.Look{}).Err(); err != nil {
			t.Errorf("syntax after Prepend in %q: %v", source, err)
		}
	}
	for _, ending := range []string{"}", "$", "#", "&", "\\", "^", "_"} {
		source := "\u0600" + ending
		if err := latex.Render(source, latex.Look{}).Err(); err == nil {
			t.Errorf("grapheme joining hid invalid syntax in %q", source)
		}
	}
	if got, want := rendered(t, "{\u0600α}"), []string{"\u0600α"}; !slices.Equal(got, want) {
		t.Fatalf("literal Prepend cluster = %q; want %q", got, want)
	}
}

func TestCrossAtomGraphemesKeepTheirSourcePlacement(t *testing.T) {
	type cell struct {
		x, y    int
		content string
		width   int
		attr    grid.Attr
	}
	for _, test := range []struct {
		source string
		width  int
		cells  []cell
	}{
		{source: "👩{\u200d💻}^2", width: 5, cells: []cell{{0, 1, "👩", 2, 0}, {2, 1, "💻", 2, 0}, {4, 0, "2", 1, 0}}},
		{source: "👩\\textbf{\u200d💻}^2", width: 5, cells: []cell{{0, 1, "👩", 2, 0}, {2, 1, "💻", 2, grid.Bold}, {4, 0, "2", 1, 0}}},
		{source: "\\frac{👩{\u200d💻}}{x}", width: 6, cells: []cell{{1, 0, "👩", 2, 0}, {3, 0, "💻", 2, 0}, {0, 1, "─", 1, 0}, {5, 1, "─", 1, 0}, {2, 2, "x", 1, 0}}},
		{source: "{\u0600\\alpha}", width: 2, cells: []cell{{0, 0, "\u0600", 1, 0}, {1, 0, "α", 1, 0}}},
		{source: "\\sqrt[\u0600]{x}", width: 3, cells: []cell{{0, 0, "\u0600", 1, 0}, {2, 0, "─", 1, 0}, {1, 1, "√", 1, 0}, {2, 1, "x", 1, 0}}},
		{source: "🇨{🇳}^2", width: 3, cells: []cell{{0, 1, "🇨", 1, 0}, {1, 1, "🇳", 1, 0}, {2, 0, "2", 1, 0}}},
		{source: "🇨{🇺🇸ा}x^1", width: 5, cells: []cell{{0, 1, "🇨", 1, 0}, {1, 1, "🇺🇸ा", 2, 0}, {3, 1, "x", 1, 0}, {4, 0, "1", 1, 0}}},
		{source: "x^1{\u0600}{界}", width: 5, cells: []cell{{0, 1, "x", 1, 0}, {1, 0, "1", 1, 0}, {2, 1, "\u0600", 1, 0}, {3, 1, "界", 2, 0}}},
		{source: "🇨{🇺🇸ा}x^1{\u0600}{界}", width: 8, cells: []cell{{0, 1, "🇨", 1, 0}, {1, 1, "🇺🇸ा", 2, 0}, {3, 1, "x", 1, 0}, {4, 0, "1", 1, 0}, {5, 1, "\u0600", 1, 0}, {6, 1, "界", 2, 0}}},
		{source: "X{🏻}^2", width: 4, cells: []cell{{0, 1, "X", 1, 0}, {1, 1, "🏻", 2, 0}, {3, 0, "2", 1, 0}}},
	} {
		t.Run(test.source, func(t *testing.T) {
			formula := latex.Render(test.source, latex.Look{})
			if formula.Err() != nil || formula.Source() != test.source || formula.Width() != test.width {
				t.Fatalf("Render(%q): source=%q width=%d err=%v", test.source, formula.Source(), formula.Width(), formula.Err())
			}
			surface := grid.NewSurface(test.width, formula.HeightForWidth(test.width))
			formula.Draw(surface.View())
			for _, want := range test.cells {
				got, _ := surface.CellAt(want.x, want.y)
				if got.Content() != want.content || got.Width() != want.width || got.Style.Attr != want.attr {
					t.Fatalf("cell (%d,%d) = %+v, want %+v", want.x, want.y, got, want)
				}
				for x := want.x + 1; x < want.x+want.width; x++ {
					continuation, _ := surface.CellAt(x, want.y)
					if continuation.Content() != "" || continuation.Width() != 0 {
						t.Fatalf("continuation (%d,%d) = %+v", x, want.y, continuation)
					}
				}
			}
			rows := formula.Rows(test.width)
			projected := grid.NewSurface(test.width, len(rows))
			for y, row := range rows {
				row.Content.Draw(projected.View(), row.Offset, y)
				if !reflect.DeepEqual(projected.Row(y), surface.Row(y)) {
					t.Fatalf("row %d projection changed atom placement", y)
				}
				for i := range row.Content {
					row.Content[i] = text.Span{Text: "caller mutation"}
				}
			}
			after := grid.NewSurface(test.width, len(rows))
			formula.Draw(after.View())
			for y := range rows {
				if !reflect.DeepEqual(after.Row(y), surface.Row(y)) {
					t.Fatalf("public Rows mutated prepared formula row %d", y)
				}
			}
		})
	}
	for _, source := range []string{"", "{}", "x{}", "{\u0301}", "\\frac{}{}", "👩‍💻^2", "\\frac{e{\u0301}}{x}", "e{\u0301}{\u0301}^2"} {
		if err := latex.Render(source, latex.Look{}).Err(); err != nil {
			t.Fatalf("representable Render(%q): %v", source, err)
		}
	}
}

func BenchmarkCrossAtomCombiningMarks(b *testing.B) {
	for _, count := range []int{256, 1024, 4096} {
		b.Run(strconv.Itoa(count), func(b *testing.B) {
			source := "e" + strings.Repeat("{\u0301}", count) + "^2"
			b.ReportAllocs()
			b.SetBytes(int64(len(source)))
			for b.Loop() {
				if err := latex.Render(source, latex.Look{}).Err(); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkLiteralGraphemeRuns(b *testing.B) {
	for _, count := range []int{256, 1024, 4096} {
		b.Run(strconv.Itoa(count), func(b *testing.B) {
			source := strings.Repeat("👩‍💻", count)
			b.ReportAllocs()
			b.SetBytes(int64(len(source)))
			for b.Loop() {
				if err := latex.Render(source, latex.Look{}).Err(); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkUnbracedGraphemeScripts(b *testing.B) {
	for _, count := range []int{256, 1024, 4096} {
		b.Run(strconv.Itoa(count), func(b *testing.B) {
			source := strings.Repeat("x^👩‍💻+", count)
			b.ReportAllocs()
			b.SetBytes(int64(len(source)))
			for b.Loop() {
				if err := latex.Render(source, latex.Look{}).Err(); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
