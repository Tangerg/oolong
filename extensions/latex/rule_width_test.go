package latex_test

import (
	"testing"

	"github.com/Tangerg/oolong/extensions/latex"
)

func TestFormulaRulesKeepTheirAssignedColumns(t *testing.T) {
	for _, test := range []struct {
		name, source, pattern, rule string
		row, width                  int
	}{
		{name: "wide fraction", source: `\frac{x}{y}`, pattern: "界", rule: "界 ", row: 1, width: 3},
		{name: "wide overline", source: `\overline{abc}`, pattern: "界", rule: "界 ", width: 3},
		{name: "whole pattern", source: `\frac{x}{y}`, pattern: "=-", rule: "=- ", row: 1, width: 3},
		{name: "zero columns", source: `\frac{x}{y}`, pattern: "\u0301", rule: "   ", row: 1, width: 3},
	} {
		t.Run(test.name, func(t *testing.T) {
			formula := latex.Render(test.source, latex.Look{Glyphs: latex.Glyphs{Horizontal: test.pattern}})
			if err := formula.Err(); err != nil {
				t.Fatal(err)
			}
			lines := formula.Lines()
			if len(lines) <= test.row || lines[test.row].String() != test.rule || formula.Width() != test.width {
				t.Fatalf("rule = %v, width = %d; want %q, width %d", lines, formula.Width(), test.rule, test.width)
			}
			if width := lines[test.row].Width(); width != test.width {
				t.Fatalf("rule occupies %d columns, want %d", width, test.width)
			}
		})
	}
}
