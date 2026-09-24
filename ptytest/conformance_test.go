package ptytest_test

import (
	"bytes"
	"image"
	"math/rand/v2"
	"slices"
	"strings"
	"testing"

	"github.com/Tangerg/oolong/core/ansi"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/ptytest"
)

// A renderer writes what turns the frame a terminal is showing into the next one,
// and the whole of its work is deciding what not to write. Nothing inside the
// renderer can check that decision: it compares its own idea of the old frame with
// its own idea of the new one, so a mistake in what it says leaves both of its ideas
// agreeing. The terminal is the only thing that can disagree, and [ptytest.Screen] is
// the terminal here.

// atoms are what a cell can hold, chosen so that the ones a renderer has to reason
// about are all in reach: two-column glyphs, clusters of several code points, and
// ordinary letters, which are what make a run long enough to be worth skipping.
var atoms = []string{"a", "b", "z", " ", "0", "界", "안", "é", "🇯🇵", "👩‍👩‍👦", "\t"}

var styles = []grid.Style{
	{},
	{Attr: grid.Bold},
	{FG: grid.RGBColor(0x80, 0, 0)},
	{BG: grid.RGBColor(0, 0x40, 0x80), Attr: grid.Underline},
}

// picture is what a frame is drawn from, kept by the test so that one frame can be a
// change to the last rather than something unrelated.
//
// Rows that move as a block are the case the renderer has a second answer for — it
// asks the terminal to scroll instead of rewriting them — and rows that arrive
// unrelated to each other never produce one.
type picture struct {
	rows   []string
	cursor grid.Cursor
}

func (p picture) draw(view grid.View) {
	for y, row := range p.rows {
		x := 0
		for cluster := range strings.SplitSeq(row, "\x00") {
			if cluster == "" {
				continue
			}
			style := styles[len(cluster)%len(styles)]
			x += max(view.Text(x, y, cluster, style), grid.ClusterWidth(cluster))
		}
	}
	if p.cursor.Visible {
		view.PlaceCursor(p.cursor.Pos.X, p.cursor.Pos.Y, p.cursor.Style)
	}
}

// next is one change to a picture: the kinds an interface actually makes.
func (p picture) next(random *rand.Rand, cols, rows int) picture {
	out := picture{rows: slices.Clone(p.rows), cursor: p.cursor}
	switch random.IntN(6) {
	case 0, 1:
		// A log that grew: everything moves up, and the bottom is new.
		by := 1 + random.IntN(3)
		out.rows = append(out.rows[min(by, len(out.rows)):], freshRows(random, by, cols)...)
	case 2:
		// A reader who scrolled back.
		by := 1 + random.IntN(3)
		out.rows = append(freshRows(random, by, cols), out.rows[:max(len(out.rows)-by, 0)]...)
	case 3:
		out.rows[random.IntN(len(out.rows))] = freshRows(random, 1, cols)[0]
	case 4:
		out.rows[random.IntN(len(out.rows))] = ""
	default:
		out.rows = freshRows(random, rows, cols)
	}
	out.cursor = grid.Cursor{
		Visible: random.IntN(3) > 0,
		Pos:     image.Pt(random.IntN(cols), random.IntN(rows)),
	}
	return out
}

func freshRows(random *rand.Rand, n, cols int) []string {
	out := make([]string, n)
	for i := range out {
		var row strings.Builder
		for width := 0; width < cols; {
			atom := atoms[random.IntN(len(atoms))]
			row.WriteString(atom)
			row.WriteString("\x00")
			width += max(grid.ClusterWidth(atom), 1)
		}
		out[i] = row.String()
	}
	return out
}

// shownAfter applies every frame the renderer produced for these pictures, and
// reports what the terminal ends up showing and whether any of those frames asked
// the terminal to scroll — which is the answer the renderer has instead of a diff,
// and the one a test that never provoked it would be passing without.
func shownAfter(t *testing.T, cols, rows int, pictures []picture) ([]string, bool) {
	t.Helper()
	renderer := grid.NewScreen(cols, rows)
	model, err := ptytest.NewScreen(ptytest.Size{Cols: cols, Rows: rows})
	if err != nil {
		t.Fatal(err)
	}
	var frame bytes.Buffer
	scrolled := false
	for i, p := range pictures {
		frame.Reset()
		p.draw(renderer.Frame())
		if err := renderer.Flush(&frame); err != nil {
			t.Fatalf("frame %d: %v", i, err)
		}
		if err := model.Apply(frame.Bytes()); err != nil {
			t.Fatalf("frame %d: the terminal could not apply it: %v", i, err)
		}
		scrolled = scrolled || asksToScroll(t, frame.Bytes())
	}
	if err := model.Flush(); err != nil {
		t.Fatal(err)
	}
	return slices.Clone(model.Rows()), scrolled
}

// asksToScroll reports whether a frame moves rows by asking the terminal to scroll.
func asksToScroll(t *testing.T, frame []byte) bool {
	t.Helper()
	var scanner ansi.Scanner
	found := false
	err := scanner.Feed(string(frame), func(piece ansi.Piece) error {
		if piece.Kind == ansi.Control && (piece.Final == 'S' || piece.Final == 'T') {
			found = true
		}
		return nil
	})
	if err != nil {
		t.Fatalf("reading the frame back: %v", err)
	}
	return found
}

func TestARendererSaysTheSameThingHoweverManyFramesItTook(t *testing.T) {
	const cols, rows = 14, 6
	scrolledSomewhere := false
	for run := range 80 {
		random := rand.New(rand.NewPCG(uint64(run), 0x6f6f6c6f6e67)) //nolint:gosec // reproducible fixtures
		sequence := []picture{{rows: freshRows(random, rows, cols)}}
		for range 6 {
			sequence = append(sequence, sequence[len(sequence)-1].next(random, cols, rows))
		}
		last := sequence[len(sequence)-1]

		incremental, scrolled := shownAfter(t, cols, rows, sequence)
		whole, _ := shownAfter(t, cols, rows, []picture{last})
		scrolledSomewhere = scrolledSomewhere || scrolled
		if !slices.Equal(incremental, whole) {
			t.Fatalf("run %d: after %d frames the terminal shows\n%s\nand after the last one alone\n%s",
				run, len(sequence), strings.Join(incremental, "\n"), strings.Join(whole, "\n"))
		}
	}
	if !scrolledSomewhere {
		t.Fatal("no frame asked the terminal to scroll, so this never reached the answer it is about")
	}
}

// TestAFullRepaintSaysWhatTheDiffAlreadySaid is the other half of it. Repainting is
// what a renderer falls back on when it no longer knows what the terminal has, so it
// has to arrive at the frame the diff arrived at.
func TestAFullRepaintSaysWhatTheDiffAlreadySaid(t *testing.T) {
	const cols, rows = 14, 6
	for run := range 80 {
		random := rand.New(rand.NewPCG(uint64(run), 0x72657061696e74)) //nolint:gosec // reproducible fixtures
		renderer := grid.NewScreen(cols, rows)
		model, err := ptytest.NewScreen(ptytest.Size{Cols: cols, Rows: rows})
		if err != nil {
			t.Fatal(err)
		}
		apply := func(p picture) {
			t.Helper()
			var frame bytes.Buffer
			p.draw(renderer.Frame())
			if err := renderer.Flush(&frame); err != nil {
				t.Fatal(err)
			}
			if err := model.Apply(frame.Bytes()); err != nil {
				t.Fatal(err)
			}
			if err := model.Flush(); err != nil {
				t.Fatal(err)
			}
		}

		current := picture{rows: freshRows(random, rows, cols)}
		apply(current)
		current = current.next(random, cols, rows)
		apply(current)
		diffed := slices.Clone(model.Rows())

		renderer.Invalidate()
		apply(current)
		if got := model.Rows(); !slices.Equal(got, diffed) {
			t.Fatalf("run %d: repainting changed what the terminal shows:\n%s\nwas\n%s",
				run, strings.Join(got, "\n"), strings.Join(diffed, "\n"))
		}
	}
}

// TestAnInlineBlockRedrawnFromNothingLooksTheSame.
//
// An inline block is written relative to where the last frame left the cursor, and
// what is above it is the terminal's own output, which the renderer cannot address at
// all. Repainting is what it does when it no longer knows where anything is, so a
// repaint has to arrive at the block the relative writes arrived at — including after
// output has been printed above it and pushed it down.
func TestAnInlineBlockRedrawnFromNothingLooksTheSame(t *testing.T) {
	const cols, rows = 14, 8
	for run := range 60 {
		random := rand.New(rand.NewPCG(uint64(run), 0x696e6c696e65)) //nolint:gosec // reproducible fixtures
		renderer := grid.NewInline(cols, 3)
		model, err := ptytest.NewScreen(ptytest.Size{Cols: cols, Rows: rows})
		if err != nil {
			t.Fatal(err)
		}
		flush := func() {
			t.Helper()
			var frame bytes.Buffer
			if err := renderer.Flush(&frame); err != nil {
				t.Fatal(err)
			}
			if err := model.Apply(frame.Bytes()); err != nil {
				t.Fatal(err)
			}
			if err := model.Flush(); err != nil {
				t.Fatal(err)
			}
		}

		var block picture
		for range 5 {
			block = picture{rows: freshRows(random, 1+random.IntN(3), cols)}
			if random.IntN(3) == 0 {
				renderer.Print(printed{rows: freshRows(random, 1, cols)})
			}
			block.draw(renderer.Frame())
			flush()
		}
		relative := slices.Clone(model.Rows())

		renderer.Invalidate()
		block.draw(renderer.Frame())
		flush()
		if got := model.Rows(); !slices.Equal(got, relative) {
			t.Fatalf("run %d: repainting the block changed the screen:\n%s\nwas\n%s",
				run, strings.Join(got, "\n"), strings.Join(relative, "\n"))
		}
	}
}

// printed is output that goes above the block and stays in the terminal's own
// scrollback.
type printed struct{ rows []string }

func (p printed) HeightForWidth(int) int { return len(p.rows) }

func (p printed) Draw(view grid.View) { picture{rows: p.rows}.draw(view) }
