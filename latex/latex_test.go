package latex_test

import (
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/layout"
	"github.com/Tangerg/oolong/latex"
)

func formulaRows(t *testing.T, formula *latex.Formula, width int) []string {
	t.Helper()
	height := formula.Measure(width)
	surface := grid.NewSurface(width, height)
	formula.Draw(surface.View())
	rows := make([]string, height)
	for y := range height {
		var row strings.Builder
		for x := range width {
			cell, ok := surface.CellAt(x, y)
			if !ok || cell.Width() == 0 {
				continue
			}
			if cell.Content == "" {
				row.WriteByte(' ')
			} else {
				row.WriteString(cell.Content)
			}
		}
		rows[y] = strings.TrimRight(row.String(), " ")
	}
	return rows
}

func TestFormulaUsesOneLayoutForMeasureDrawAndRows(t *testing.T) {
	formula := latex.Render(`x = \frac{a+b}{c}`, latex.Look{})
	if err := formula.Err(); err != nil {
		t.Fatalf("Render: %v", err)
	}

	drawn := formulaRows(t, formula, 40)
	selected := formula.Rows(40)
	if len(drawn) != formula.Measure(40) || len(selected) != len(drawn) {
		t.Fatalf("measure=%d, drawn=%d, selected=%d", formula.Measure(40), len(drawn), len(selected))
	}
	for i, row := range selected {
		if got := strings.Repeat(" ", row.Offset) + row.Text; got != drawn[i] {
			t.Fatalf("row %d selection %q != drawing %q", i, got, drawn[i])
		}
	}
	if !slices.Equal(drawn, []string{"     a + b", "x = ───────", "       c"}) {
		t.Fatalf("fraction:\n%s", strings.Join(drawn, "\n"))
	}
}

func TestFormulaRendersSymbolsRootsAndScripts(t *testing.T) {
	formula := latex.Render(`\sqrt{x_1^2 + \alpha} \leq \infty`, latex.Look{})
	if err := formula.Err(); err != nil {
		t.Fatalf("Render: %v", err)
	}
	text := strings.Join(formulaRows(t, formula, 80), "\n")
	for _, want := range []string{"√", "α", "≤", "∞", "1", "2"} {
		if !strings.Contains(text, want) {
			t.Errorf("rendering %q does not contain %q", text, want)
		}
	}
}

func TestLookStylesTextRulesAndFallbackIndependently(t *testing.T) {
	textStyle := grid.Style{Attr: grid.Bold}
	ruleStyle := grid.Style{Attr: grid.Underline}
	errorStyle := grid.Style{Attr: grid.Strike}
	look := latex.Look{Text: textStyle, Rule: ruleStyle, Error: errorStyle}

	lines := latex.Render(`\frac{x}{y}`, look).Lines()
	if len(lines) != 3 || len(lines[0]) < 2 || lines[0][1].Text != "x" || lines[0][1].Style != textStyle {
		t.Fatalf("numerator style = %+v", lines)
	}
	if len(lines[1]) == 0 || lines[1][0].Style != ruleStyle {
		t.Fatalf("rule style = %+v", lines[1])
	}
	fallback := latex.Render(`\frac{`, look).Lines()
	if len(fallback) != 1 || len(fallback[0]) != 1 || fallback[0][0].Style != errorStyle {
		t.Fatalf("fallback style = %+v", fallback)
	}
}

func TestAlignmentIsSharedByDrawingAndRows(t *testing.T) {
	formula := latex.Render("x", latex.Look{Align: layout.End})
	rows := formula.Rows(5)
	if len(rows) != 1 || rows[0].Offset != 4 {
		t.Fatalf("Rows(5) = %+v", rows)
	}
	if got := formulaRows(t, formula, 5); !slices.Equal(got, []string{"    x"}) {
		t.Fatalf("drawing = %q", got)
	}
}

func TestUnsupportedFormulaStaysReadable(t *testing.T) {
	const source = `\frac{`
	formula := latex.Render(source, latex.Look{})
	if formula.Err() == nil {
		t.Fatal("Err is nil for incomplete input")
	}
	if got := formulaRows(t, formula, 40); !slices.Equal(got, []string{source}) {
		t.Fatalf("fallback = %q, want source", got)
	}
	if formula.Source() != source {
		t.Fatalf("Source = %q", formula.Source())
	}
}

func TestHostileSourceIsRejectedBeforeTheExternalParser(t *testing.T) {
	for _, source := range []string{
		string([]byte{'x', 0}),
		string([]byte{'x', 0xff}),
		strings.Repeat("{", 257) + strings.Repeat("}", 257),
		strings.Repeat("^", 257) + "x",
		"x}",
	} {
		if err := latex.Render(source, latex.Look{}).Err(); err == nil {
			t.Errorf("Render(%q).Err is nil", source)
		}
	}
}

func TestEscapedBracesAreSymbolsRatherThanGroups(t *testing.T) {
	formula := latex.Render(`\{x\}`, latex.Look{})
	if err := formula.Err(); err != nil {
		t.Fatalf("Render: %v", err)
	}
	if got := strings.Join(formulaRows(t, formula, 20), "\n"); got != "{x}" {
		t.Fatalf("braces = %q", got)
	}
}

func TestASCIIContainsOnlyASCII(t *testing.T) {
	formula := latex.Render(`\frac{\alpha_1}{\beta^2} \leq \infty`, latex.Look{Glyphs: latex.ASCII()})
	if err := formula.Err(); err != nil {
		t.Fatalf("Render: %v", err)
	}
	for _, line := range formula.Lines() {
		for _, r := range line.String() {
			if r >= utf8.RuneSelf {
				t.Fatalf("ASCII rendering contains %q in %q", r, line.String())
			}
		}
	}
}

func TestLinesReturnsAnOwnedSnapshot(t *testing.T) {
	formula := latex.Render("x", latex.Look{})
	first := formula.Lines()
	first[0][0].Text = "changed"
	if got := formula.Lines()[0].String(); got != "x" {
		t.Fatalf("formula changed through Lines to %q", got)
	}
}

func TestNilFormulaIsEmpty(t *testing.T) {
	var formula *latex.Formula
	if formula.Source() != "" || formula.Err() != nil || formula.Width() != 0 || formula.Measure(20) != 0 || formula.Lines() != nil || formula.Rows(20) != nil {
		t.Fatal("nil Formula is not empty")
	}
	formula.Draw(grid.View{})
}
