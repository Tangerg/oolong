package grid_test

import (
	"io"
	"strings"
	"testing"

	"github.com/Tangerg/oolong/core/grid"
)

// The positioning claims three things about what a frame costs, and none of them
// was measured. These measure them.
//
// What is being compared is not another library — that would need one — but this
// library against itself: a frame that changed nothing against one that changed a
// row, and a cell diff against the full repaint it replaces. Those are the numbers
// the design rests on.

// paragraph fills a screen the way a transcript does.
func paragraph(s *grid.Screen) {
	v := s.Frame()
	w, h := s.Size()
	line := strings.Repeat("the quick brown fox jumps over the lazy dog ", 4)
	for y := range h {
		v.Text(0, y, line[y%20:y%20+min(w, 60)], grid.Style{
			FG: grid.RGBColor(uint8(y*7), 0x80, 0xC0),
		})
	}
}

func BenchmarkFrameThatChangedNothing(b *testing.B) {
	// The claim: an idle interface is silent on the wire. What this measures is
	// what silence costs, which is the diff of a screen against itself.
	s := grid.NewScreen(120, 40)
	paragraph(s)
	if err := s.Flush(io.Discard); err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	for b.Loop() {
		paragraph(s)
		_ = s.Flush(io.Discard)
	}
}

func BenchmarkFrameThatChangedOneRow(b *testing.B) {
	// A streamed token changes one row. This is what that costs against a screen
	// the terminal is already showing.
	s := grid.NewScreen(120, 40)
	paragraph(s)
	_ = s.Flush(io.Discard)

	b.ReportAllocs()
	i := 0
	for b.Loop() {
		v := s.Frame()
		paragraph(s)
		i++
		v.Text(0, 20, strings.Repeat("x", i%40+1), grid.Style{})
		_ = s.Flush(io.Discard)
	}
}

func BenchmarkFullRepaint(b *testing.B) {
	// What the diff is being compared against: every cell, every frame.
	s := grid.NewScreen(120, 40)

	b.ReportAllocs()
	for b.Loop() {
		s.Invalidate()
		paragraph(s)
		_ = s.Flush(io.Discard)
	}
}

func BenchmarkDrawingText(b *testing.B) {
	// Every frame goes through this, and a grapheme walk per cell is the price of
	// never splitting a wide cluster.
	s := grid.NewSurface(120, 40)
	const line = "the quick brown fox 中文 with a combining é and an emoji 🙂"

	b.ReportAllocs()
	for b.Loop() {
		v := s.View()
		for y := range 40 {
			v.Text(0, y, line, grid.Style{})
		}
	}
}

func BenchmarkEncodeRow(b *testing.B) {
	s := grid.NewSurface(120, 1)
	s.View().Text(0, 0, strings.Repeat("ab", 60), grid.Style{FG: grid.RGBColor(1, 2, 3)})
	row := s.Row(0)

	b.ReportAllocs()
	for b.Loop() {
		_ = grid.EncodeRow(row, grid.TrueColor)
	}
}
