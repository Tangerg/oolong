package markdown_test

import (
	"slices"
	"testing"

	"github.com/Tangerg/oolong/extensions/markdown"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/text"
)

func TestDisplayMathUsesTheConsumerRenderer(t *testing.T) {
	var source string
	appearance := look()
	appearance.SetRenderer(markdown.DisplayMath, func(_, got string) (grid.Drawable, error) {
		source = got
		return text.NewBlock(text.BlockConfig{Lines: []text.Line{
			text.Of("numerator", grid.Style{}),
			text.Of("─────────", grid.Style{}),
			text.Of("denominator", grid.Style{}),
		}}), nil
	})
	blocks := mustRender(t, "before\n\n$$\n\\frac{a}{b}\n$$\n\nafter", appearance)

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
	appearance.SetRenderer(markdown.DisplayMath, func(_, source string) (grid.Drawable, error) {
		calls = append(calls, source)
		return text.NewBlock(text.BlockConfig{Lines: []text.Line{text.Of("formula", grid.Style{})}}), nil
	})
	blocks := mustRender(t, "```math\nx^2\n```\n\n$$\ny^2\n$$", appearance)

	if !slices.Equal(calls, []string{"x^2", "y^2"}) {
		t.Fatalf("math calls = %q", calls)
	}
	equal(t, rows(t, 20, blocks), []string{"formula", "", "formula"})
}

func TestMathWithoutARendererShowsItsSource(t *testing.T) {
	equal(t, render(t, 20, "$$\nx^2 + y^2\n$$"), []string{"x^2 + y^2"})
}

func TestEmptyMathRenderingIsDifferentFromDeclining(t *testing.T) {
	appearance := look()
	appearance.SetRenderer(markdown.DisplayMath, func(string, string) (grid.Drawable, error) {
		return text.NewBlock(text.BlockConfig{Lines: []text.Line{}}), nil
	})
	blocks := mustRender(t, "$$\nx^2 + y^2\n$$", appearance)

	if len(blocks) != 1 {
		t.Fatalf("blocks = %d, want 1", len(blocks))
	}
	if got := blocks[0].HeightForWidth(20); got != 0 {
		t.Fatalf("empty rendering measures %d rows, want 0", got)
	}
	if got := blocks[0].Rows(20); len(got) != 0 {
		t.Fatalf("empty rendering has %d rows, want 0", len(got))
	}
}

func TestDisplayMathDelimitersOccupyTheirOwnLines(t *testing.T) {
	appearance := look()
	appearance.SetRenderer(markdown.DisplayMath, func(string, string) (grid.Drawable, error) {
		return text.NewBlock(text.BlockConfig{Lines: []text.Line{text.Of("formula", grid.Style{})}}), nil
	})
	// Inline dollars and four-space indentation remain ordinary Markdown. Treating
	// either as a display extension would make prose or indented code disappear.
	equal(t, rows(t, 30, mustRender(t, "before $$ x $$ after", appearance)), []string{"before $$ x $$ after"})
	equal(t, rows(t, 30, mustRender(t, "    $$\n    x\n    $$", appearance)), []string{"$$", "x", "$$"})
}

func TestMathLayoutIsClippedRatherThanReflowed(t *testing.T) {
	appearance := look()
	appearance.SetRenderer(markdown.DisplayMath, func(string, string) (grid.Drawable, error) {
		return text.NewBlock(text.BlockConfig{Lines: []text.Line{text.Of("abcdef", grid.Style{})}}), nil
	})
	blocks := mustRender(t, "$$\nx\n$$", appearance)

	if got := blocks[0].HeightForWidth(3); got != 1 {
		t.Fatalf("HeightForWidth(3) = %d, want one fixed row", got)
	}
	equal(t, rows(t, 3, blocks), []string{"abc"})
}

func TestMathRendererOutputIsOwned(t *testing.T) {
	line := text.Of("formula", grid.Style{})
	appearance := look()
	appearance.SetRenderer(markdown.DisplayMath, func(string, string) (grid.Drawable, error) {
		return text.NewBlock(text.BlockConfig{Lines: []text.Line{line}}), nil
	})
	blocks := mustRender(t, "$$\nx\n$$", appearance)
	line[0].Text = "changed"

	equal(t, rows(t, 20, blocks), []string{"formula"})
}
