package markdown_test

import (
	"slices"
	"testing"

	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/text"
	"github.com/Tangerg/oolong/markdown"
)

func TestDisplayMathUsesTheConsumerRenderer(t *testing.T) {
	var source string
	appearance := look()
	appearance.SetRenderer(markdown.DisplayMath, func(_, got string) []text.Line {
		source = got
		return []text.Line{
			text.Of("numerator", grid.Style{}),
			text.Of("─────────", grid.Style{}),
			text.Of("denominator", grid.Style{}),
		}
	})
	blocks := markdown.Render("before\n\n$$\n\\frac{a}{b}\n$$\n\nafter", appearance)

	if source != `\frac{a}{b}` {
		t.Fatalf("math source = %q", source)
	}
	equal(t, rows(t, 20, blocks), []string{
		"before", "", "numerator", "─────────", "denominator", "", "after",
	})
}

func TestMathFenceUsesTheSameRenderer(t *testing.T) {
	var calls []string
	appearance := look()
	appearance.SetRenderer(markdown.DisplayMath, func(_, source string) []text.Line {
		calls = append(calls, source)
		return []text.Line{text.Of("formula", grid.Style{})}
	})
	blocks := markdown.Render("```math\nx^2\n```\n\n$$\ny^2\n$$", appearance)

	if !slices.Equal(calls, []string{"x^2", "y^2"}) {
		t.Fatalf("math calls = %q", calls)
	}
	equal(t, rows(t, 20, blocks), []string{"formula", "", "formula"})
}

func TestMathWithoutARendererShowsItsSource(t *testing.T) {
	equal(t, render(t, 20, "$$\nx^2 + y^2\n$$"), []string{"x^2 + y^2"})
}

func TestDisplayMathDelimitersOccupyTheirOwnLines(t *testing.T) {
	appearance := look()
	appearance.SetRenderer(markdown.DisplayMath, func(string, string) []text.Line {
		return []text.Line{text.Of("formula", grid.Style{})}
	})
	// Inline dollars and four-space indentation remain ordinary Markdown. Treating
	// either as a display extension would make prose or indented code disappear.
	equal(t, rows(t, 30, markdown.Render("before $$ x $$ after", appearance)), []string{"before $$ x $$ after"})
	equal(t, rows(t, 30, markdown.Render("    $$\n    x\n    $$", appearance)), []string{"$$", "x", "$$"})
}

func TestMathLayoutIsClippedRatherThanReflowed(t *testing.T) {
	appearance := look()
	appearance.SetRenderer(markdown.DisplayMath, func(string, string) []text.Line {
		return []text.Line{text.Of("abcdef", grid.Style{})}
	})
	blocks := markdown.Render("$$\nx\n$$", appearance)

	if got := blocks[0].Measure(3); got != 1 {
		t.Fatalf("Measure(3) = %d, want one fixed row", got)
	}
	equal(t, rows(t, 3, blocks), []string{"abc"})
}

func TestMathRendererOutputIsOwned(t *testing.T) {
	line := text.Of("formula", grid.Style{})
	appearance := look()
	appearance.SetRenderer(markdown.DisplayMath, func(string, string) []text.Line {
		return []text.Line{line}
	})
	blocks := markdown.Render("$$\nx\n$$", appearance)
	line[0].Text = "changed"

	equal(t, rows(t, 20, blocks), []string{"formula"})
}

func TestStreamOwnsItsExtensionRegistry(t *testing.T) {
	first := func(string, string) []text.Line { return []text.Line{text.Of("first", grid.Style{})} }
	second := func(string, string) []text.Line { return []text.Line{text.Of("second", grid.Style{})} }
	appearance := markdown.Look{}
	appearance.SetRenderer(markdown.DisplayMath, first)
	changed := appearance
	changed.SetRenderer(markdown.DisplayMath, second)
	equal(t, rows(t, 20, markdown.Render("$$\nx\n$$", appearance)), []string{"first"})
	changed.SetRenderer(markdown.DisplayMath, nil)
	equal(t, rows(t, 20, markdown.Render("$$\nx\n$$", changed)), []string{"x"})

	var stream markdown.Stream
	stream.SetLook(appearance)
	stream.Feed("$$\nx\n$$")

	appearance.SetRenderer(markdown.DisplayMath, second)
	snapshot := stream.Look()
	snapshot.SetRenderer(markdown.DisplayMath, second)
	equal(t, rows(t, 20, stream.Open()), []string{"first"})
}

func TestStreamDoesNotPublishInsideDisplayMath(t *testing.T) {
	appearance := look()
	var source string
	appearance.SetRenderer(markdown.DisplayMath, func(_, got string) []text.Line {
		source = got
		return []text.Line{text.Of("formula", grid.Style{})}
	})
	var stream markdown.Stream
	stream.SetLook(appearance)

	if done := stream.Feed("$$\na\n\n"); done != nil {
		t.Fatalf("published inside formula: %+v", done)
	}
	if open := stream.Open(); len(open) != 1 {
		t.Fatalf("open formula blocks = %d, want 1", len(open))
	}
	done := stream.Feed("b\n$$\n\nafter\n")
	if len(done) != 1 {
		t.Fatalf("published blocks = %d, want formula only", len(done))
	}
	if source != "a\n\nb" {
		t.Fatalf("published source = %q", source)
	}
	equal(t, rows(t, 20, done), []string{"formula"})
}
