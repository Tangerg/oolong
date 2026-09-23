package latex_test

import (
	"strings"
	"testing"

	"github.com/Tangerg/oolong/latex"
)

// TestRenderingGrowsWithTheFormulaAndNotWithItsSquare holds the row builder to being
// a builder.
//
// A row is assembled one cell at a time and adjacent cells usually share a style.
// Merging them by appending to the last span's string copies everything written so
// far on every cell, so a formula twice as long costs four times as much: 56 KiB of
// "a+a+a+…" allocated gigabytes to draw a few thousand columns. The ratio is what is
// measured, because the constant is a property of the machine and the quadratic is
// not.
func TestRenderingGrowsWithTheFormulaAndNotWithItsSquare(t *testing.T) {
	formula := func(terms int) string { return strings.Repeat("a+", terms) + "a" }
	cost := func(terms int) float64 {
		return testing.AllocsPerRun(1, func() { latex.Render(formula(terms), latex.Look{}) })
	}

	small := cost(2000)
	large := cost(4000)
	if small <= 0 {
		t.Fatal("rendering allocated nothing, so this measures nothing")
	}
	// Twice the formula, at most three times the allocations. Quadratic growth is
	// four, and the slack is for the fixed cost of a parse.
	if ratio := large / small; ratio > 3 {
		t.Fatalf("doubling the formula multiplied allocations by %.1f, want linear growth", ratio)
	}
}
