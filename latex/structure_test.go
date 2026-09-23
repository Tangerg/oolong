package latex_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/Tangerg/oolong/core/grid"

	"github.com/Tangerg/oolong/latex"
)

// The two-dimensional constructions, each checked as what a reader sees rather than
// as the boxes it was built from. Placement is the whole content of these macros: a
// binomial that lost its brackets, an annotation beside its relation instead of over
// it, or a root index where a power goes are all still "rendered", and all wrong.

func rendered(t *testing.T, source string) []string {
	t.Helper()
	formula := latex.Render(source, latex.Look{})
	if err := formula.Err(); err != nil {
		t.Fatalf("Render(%q): %v", source, err)
	}
	return formulaRows(t, formula, 40)
}

func TestABinomialIsBracketedAndStacked(t *testing.T) {
	want := []string{
		"⎛n⎞",
		"⎜ ⎟",
		"⎝k⎠",
	}
	if got := rendered(t, `\binom{n}{k}`); !slices.Equal(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestABinomialIsNotTheFractionOfItsTerms(t *testing.T) {
	// What the two rows above already pin is the binomial; this is the one thing that
	// has to stay different about it, stated where a change would break it.
	binomial := rendered(t, `\binom{n}{k}`)
	fraction := rendered(t, `\frac{n}{k}`)
	if slices.Equal(binomial, fraction) {
		t.Fatalf("a binomial rendered as its fraction: %q", binomial)
	}
	if len(fraction) != 3 || !strings.HasPrefix(fraction[1], "─") {
		t.Fatalf("the fraction under test has no rule to differ by: %q", fraction)
	}
}

func TestAnOverlineCoversExactlyWhatItIsOver(t *testing.T) {
	want := []string{
		"─────",
		"x + y",
	}
	if got := rendered(t, `\overline{x+y}`); !slices.Equal(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestAnOverlineLeavesItsNeighboursOnTheBaseline(t *testing.T) {
	want := []string{
		" ──",
		"axyb",
	}
	if got := rendered(t, `a \overline{xy} b`); !slices.Equal(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestAStackedRelationSitsOverItsRelationRatherThanBesideIt(t *testing.T) {
	// A superscript is beside its base and reads as a second symbol; an annotation
	// belongs over the relation it qualifies so the two read as one.
	want := []string{
		" def",
		"a = b",
	}
	if got := rendered(t, `a \stackrel{def}{=} b`); !slices.Equal(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
	if power := rendered(t, `a =^{def} b`); slices.Equal(power, want) {
		t.Fatalf("a stacked relation rendered as a power: %q", power)
	}
}

func TestARootIndexIsNotWhereAPowerWouldBe(t *testing.T) {
	// Placed after the sign it would read as a cube rather than a cube root.
	cubeRoot := rendered(t, `\sqrt[3]{x}`)
	if len(cubeRoot) != 2 {
		t.Fatalf("got %q, want an index row over the radical", cubeRoot)
	}
	plain := rendered(t, `\sqrt{x}`)
	if slices.Equal(cubeRoot, plain) {
		t.Fatalf("the index was dropped: %q", cubeRoot)
	}
	if got, want := cubeRoot[0][:len("3")], "3"; got != want {
		t.Fatalf("index row = %q, want it to begin with the index", cubeRoot[0])
	}
	if last := cubeRoot[1]; last[len(last)-1:] == "3" {
		t.Fatalf("the index followed the radicand like a power: %q", cubeRoot)
	}
}

func TestAStructuralMacroReportsTheArgumentsItNeeds(t *testing.T) {
	for _, source := range []string{`\binom{n}`, `\stackrel{def}`, `\sqrt`} {
		if err := latex.Render(source, latex.Look{}).Err(); err == nil {
			t.Errorf("Render(%q) reported no error", source)
		}
	}
}

func TestEveryReportedFailureNamesThisPackage(t *testing.T) {
	// An explanation reaching an application says where it came from. The errors the
	// renderer raises internally are phrased for a reader of a formula, so the name
	// has to be added on the way out rather than written into each of them.
	for _, source := range []string{
		`\binom{n}`, `\stackrel{def}`, `\sqrt`, `x^`, `{a`, `a}`,
		`\unknownmacro{x}`, "\xff", `\frac{1}`,
	} {
		err := latex.Render(source, latex.Look{}).Err()
		if err == nil {
			t.Errorf("Render(%q) reported no error", source)
			continue
		}
		if !strings.HasPrefix(err.Error(), "latex: ") {
			t.Errorf("Render(%q) = %q, want it to name the package", source, err)
		}
	}
}

func TestNoFailureNamesTheDelimiterThisPackageAdds(t *testing.T) {
	// The expression is wrapped in math delimiters before it reaches the parser, so a
	// complaint quoting one describes a character the caller never typed. Saying the
	// expression ended early says the same thing about the source they wrote.
	for _, source := range []string{`\frac{1}`, `\binom{1}`, `\sqrt`, `\stackrel{d}`, `\overline`} {
		err := latex.Render(source, latex.Look{}).Err()
		if err == nil {
			t.Errorf("Render(%q) reported no error", source)
			continue
		}
		if strings.Contains(err.Error(), "$") {
			t.Errorf("Render(%q) = %q, which names the wrapper rather than the source", source, err)
		}
	}
}

func TestADelimiterInTheSourceIsRefused(t *testing.T) {
	// Refusing it is what makes the rule above exact: after this, a delimiter in any
	// message can only have come from the wrapper.
	for _, source := range []string{`$x$`, `a $ b`, `x + $`} {
		err := latex.Render(source, latex.Look{}).Err()
		if err == nil || !strings.Contains(err.Error(), "math delimiters") {
			t.Errorf("Render(%q) = %v, want it refused alongside the other delimiters", source, err)
		}
	}
}

func TestAGroupAfterAStructuralMacroIsItsNeighbourAndNotItsArgument(t *testing.T) {
	// TeX reads exactly as many groups as the macro takes; the next one stands beside
	// the result. Refusing it as a third argument would reject valid source.
	got := rendered(t, `\binom{n}{k}c`)
	if len(got) != 3 || !strings.HasSuffix(got[1], "c") {
		t.Fatalf("got %q, want the neighbour beside the binomial", got)
	}
}

func TestFontMacrosKeepTheirArgumentAndDropTheirFont(t *testing.T) {
	// A terminal has one typeface. What survives is emphasis, which the cell carries,
	// and the argument itself; the font name is not a thing a cell can be.
	for _, source := range []string{
		`\mathbf{ab}`, `\textbf{ab}`, `\mathit{ab}`, `\mathtt{ab}`,
		`\mathcal{ab}`, `\mathbb{ab}`, `\mathfrak{ab}`, `\operatorname{ab}`,
	} {
		want := []string{"ab"}
		if got := rendered(t, source); !slices.Equal(got, want) {
			t.Errorf("Render(%q) = %q, want %q", source, got, want)
		}
	}
}

func TestEmphasisMacrosStyleTheCellsTheyCover(t *testing.T) {
	// The argument reads the same either way, so the only observable difference is
	// the attribute the cell carries.
	for _, tc := range []struct {
		source string
		want   grid.Attr
	}{
		{`\mathbf{ab}`, grid.Bold},
		{`\textbf{ab}`, grid.Bold},
		{`\mathit{ab}`, grid.Italic},
		{`\textit{ab}`, grid.Italic},
	} {
		formula := latex.Render(tc.source, latex.Look{})
		if err := formula.Err(); err != nil {
			t.Fatalf("Render(%q): %v", tc.source, err)
		}
		surface := grid.NewSurface(10, formula.HeightForWidth(10))
		formula.Draw(surface.View())
		cell, ok := surface.CellAt(0, 0)
		if !ok {
			t.Fatalf("Render(%q) drew nothing", tc.source)
		}
		if cell.Style.Attr&tc.want == 0 {
			t.Errorf("Render(%q) first cell attr = %v, want %v set",
				tc.source, cell.Style.Attr, tc.want)
		}
	}
	plain := latex.Render(`ab`, latex.Look{})
	surface := grid.NewSurface(10, plain.HeightForWidth(10))
	plain.Draw(surface.View())
	if cell, ok := surface.CellAt(0, 0); ok && cell.Style.Attr != 0 {
		t.Fatalf("plain text carried attr %v", cell.Style.Attr)
	}
}

func TestSpacingMacrosOccupyTheRoomTheyName(t *testing.T) {
	for _, tc := range []struct {
		source string
		want   string
	}{
		{`a \, b`, "a b"},
		{`a \quad b`, "a  b"},
		{`a \qquad b`, "a    b"},
		{`a \! b`, "ab"},
	} {
		got := rendered(t, tc.source)
		if !slices.Equal(got, []string{tc.want}) {
			t.Errorf("Render(%q) = %q, want %q", tc.source, got, []string{tc.want})
		}
	}
}
