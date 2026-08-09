package kit_test

import (
	"strings"
	"testing"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/text"
)

// BenchmarkParagraphClippedDraw keeps the retained content large while the visible
// terminal window stays fixed. A viewport projects a full-height child view through a
// small clip, so steady-state drawing should be paid for by visible rows rather than
// by the rows hidden above and below it.
func BenchmarkParagraphClippedDraw(b *testing.B) {
	const (
		rows    = 10_000
		visible = 40
	)
	paragraph := kit.NewParagraph(strings.Repeat("content\n", rows), grid.Style{})
	height := paragraph.Measure(80)
	surface := grid.NewSurface(80, visible)
	view := surface.View().Sub(grid.Rect(0, -height/2, 80, height))

	b.ReportAllocs()
	b.ReportMetric(visible, "visible-rows/op")
	b.ResetTimer()
	for b.Loop() {
		paragraph.Draw(view)
	}
}

func BenchmarkCodeClippedDraw(b *testing.B) {
	const (
		rows    = 10_000
		visible = 40
	)
	lines := make([]text.Line, rows)
	for i := range lines {
		lines[i] = text.Of("package content", grid.Style{})
	}
	code := kit.NewCode(lines)
	code.Gutter = kit.LineNumbers{Separator: "│"}
	height := code.Measure(80)
	surface := grid.NewSurface(80, visible)
	view := surface.View().Sub(grid.Rect(0, -height/2, 80, height))

	b.ReportAllocs()
	b.ReportMetric(visible, "visible-rows/op")
	b.ResetTimer()
	for b.Loop() {
		code.Draw(view)
	}
}

func BenchmarkPaletteClippedDraw(b *testing.B) {
	const (
		rows    = 10_000
		visible = 40
	)
	found := make([]headless.Found, rows)
	for i := range found {
		found[i] = headless.Found{
			Command: headless.Command{Name: "inspect-session", Title: "Inspect a running session"},
			At:      []int{0, 2, 5},
		}
	}
	palette := kit.Palette{Found: found, Selected: rows / 2}
	surface := grid.NewSurface(80, visible)
	view := surface.View().Sub(grid.Rect(0, -rows/2, 80, rows))

	b.ReportAllocs()
	b.ReportMetric(visible, "visible-rows/op")
	b.ResetTimer()
	for b.Loop() {
		palette.Draw(view)
	}
}

func BenchmarkMessageClippedDraw(b *testing.B) {
	const (
		rows    = 10_000
		visible = 40
	)
	message := kit.Message{Speaker: "assistant", Body: strings.Repeat("content\n", rows)}
	height := message.Measure(80)
	surface := grid.NewSurface(80, visible)
	view := surface.View().Sub(grid.Rect(0, -height/2, 80, height))

	b.ReportAllocs()
	b.ReportMetric(visible, "visible-rows/op")
	b.ResetTimer()
	for b.Loop() {
		message.Draw(view)
	}
}
