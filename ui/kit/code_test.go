package kit_test

import (
	"math"
	"reflect"
	"testing"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/text"
	"github.com/Tangerg/oolong/ui/kit"
)

func TestCodeNumbersLogicalLinesAndLeavesContinuationsBlank(t *testing.T) {
	numbers := &kit.LineNumbers{Separator: "│"}
	code := kit.NewCode([]text.Line{
		text.Of("alpha beta", grid.Style{}),
		text.Of("x", grid.Style{}),
	})
	code.Gutter = numbers

	if got := code.HeightForWidth(8); got != 3 {
		t.Fatalf("HeightForWidth(8) = %d, want 3 wrapped rows", got)
	}
	equalRows(t, paint(8, 3, code.Draw), []string{
		"1│.alpha",
		"...beta.",
		"2│.x....",
	})

	rows := code.Rows(8)
	if got := []int{rows[0].SourceLine, rows[1].SourceLine, rows[2].SourceLine}; !reflect.DeepEqual(got, []int{1, 1, 2}) {
		t.Fatalf("source lines = %v, want [1 1 2]", got)
	}
	if !rows[1].Joined || rows[0].Offset != 3 {
		t.Fatalf("rows = %#v, want a continuation after a three-column gutter", rows)
	}
}

func TestCodeOwnsItsStyledInput(t *testing.T) {
	lines := []text.Line{{{Text: "before"}}}
	code := kit.NewCode(lines)
	lines[0][0].Text = "after"

	if got := code.Lines()[0].String(); got != "before" {
		t.Fatalf("Lines()[0] = %q, want owned input", got)
	}
}

func TestLineNumbersCanStartAtASourceOffset(t *testing.T) {
	numbers := kit.LineNumbers{First: 98, Separator: "│"}
	if got := numbers.Width(3); got != 5 {
		t.Fatalf("Width(3) = %d, want three digits, separator, and gap", got)
	}

	rows := []text.Row{
		{SourceLine: 1},
		{SourceLine: 1, Joined: true},
		{SourceLine: 2},
		{SourceLine: 3},
	}
	equalRows(t, paint(5, 4, func(v grid.View) { numbers.Draw(v, rows) }), []string{
		" 98│.",
		".....",
		" 99│.",
		"100│.",
	})
}

func TestLineNumbersUseOnlyTheVisiblePadding(t *testing.T) {
	screen := grid.NewSurface(2, 1)
	screen.View().Text(0, 0, "xx", grid.Style{})
	wide := screen.View().Sub(grid.Area(0, 0, math.MaxInt, 1))
	kit.LineNumbers{}.Draw(wide, []text.Row{{SourceLine: 1}})
	if got := rowOf(screen.View(), 0, 2); got != "  " {
		t.Fatalf("visible padding = %q, want two empty cells", got)
	}
}
