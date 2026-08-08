package kit_test

import (
	"reflect"
	"testing"

	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/text"
)

func TestCodeNumbersLogicalLinesAndLeavesContinuationsBlank(t *testing.T) {
	numbers := &kit.LineNumbers{Separator: "│"}
	code := kit.NewCode([]text.Line{
		text.Of("alpha beta", grid.Style{}),
		text.Of("x", grid.Style{}),
	})
	code.Gutter = numbers

	if got := code.Measure(8); got != 3 {
		t.Fatalf("Measure(8) = %d, want 3 wrapped rows", got)
	}
	equalRows(t, paint(8, 3, code.Draw), []string{
		"1│.alpha",
		"...beta.",
		"2│.x....",
	})

	rows := code.Rows(8)
	if got := []int{rows[0].Line, rows[1].Line, rows[2].Line}; !reflect.DeepEqual(got, []int{1, 1, 2}) {
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
		{Line: 1},
		{Line: 1, Joined: true},
		{Line: 2},
		{Line: 3},
	}
	equalRows(t, paint(5, 4, func(v grid.View) { numbers.Draw(v, rows) }), []string{
		" 98│.",
		".....",
		" 99│.",
		"100│.",
	})
}
