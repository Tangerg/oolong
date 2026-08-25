package grid_test

import (
	"fmt"

	"github.com/Tangerg/oolong/core/grid"
)

// Render is the way out of the grid for a caller with no terminal — a test, a run
// under a build server, output being piped to a file. Everything above this package
// draws into a View, so this is where drawing becomes something you can read.
func ExampleRender() {
	rows := grid.Render(20, 3, func(v grid.View) {
		v.Text(0, 0, "oolong", grid.Style{Attr: grid.Bold})
		v.Text(0, 1, "a terminal library", grid.Style{})
	})
	for _, row := range rows {
		fmt.Printf("%q\n", row)
	}
	// Output:
	// "oolong"
	// "a terminal library"
	// ""
}

// A view is a clipped window addressed in its own coordinates: a widget handed one
// draws from (0, 0) and cannot reach outside its box, so nothing has to be told where
// on screen it ended up.
func ExampleView_Sub() {
	rows := grid.Render(24, 2, func(v grid.View) {
		right := v.Sub(grid.Rect(12, 0, 12, 2))
		// Local coordinates: (0, 0) is the left edge of the sub-view, not the screen.
		right.Text(0, 0, "right half", grid.Style{})
		// Discarded rather than reported: the box is a boundary, not a convention a
		// widget could break by accident and see on screen.
		right.Text(0, 5, "below", grid.Style{})
	})
	for _, row := range rows {
		fmt.Printf("%q\n", row)
	}
	// Output:
	// "            right half"
	// ""
}

// A multi-column cluster is never split. One that would straddle an edge is dropped
// and its columns are blanked, because half a glyph is worse than a gap.
func ExampleView_Text() {
	rows := grid.Render(5, 2, func(v grid.View) {
		// Widths chosen so the last cluster has one column and needs two.
		v.Text(0, 0, "ab中", grid.Style{})
		v.Text(0, 1, "中中中", grid.Style{})
	})
	for _, row := range rows {
		fmt.Printf("%q\n", row)
	}
	// Output:
	// "ab中"
	// "中中"
}

// EncodeRow turns one finished row into inline terminal text: styles and hyperlinks,
// and nothing that moves the cursor or erases anything. It is how a transcript line is
// printed into the terminal's own scrollback, where it has to survive on its own.
func ExampleEncodeRow() {
	s := grid.NewSurface(12, 1)
	v := s.View()
	n := v.Text(0, 0, "see ", grid.Style{})
	v.Text(n, 0, "docs", grid.Style{Attr: grid.Underline})
	v.Link(n, 0, 4, "https://example.test")

	fmt.Printf("%q\n", grid.EncodeRow(s.Row(0), grid.TrueColor))
	// Output:
	// "see \x1b[0;4m\x1b]8;;https://example.test\x1b\\docs\x1b]8;;\x1b\\\x1b[0m"
}
