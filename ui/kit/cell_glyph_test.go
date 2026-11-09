package kit_test

import (
	"errors"
	"testing"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/diff"
	"github.com/Tangerg/oolong/ui/internal/scenetest"
	"github.com/Tangerg/oolong/ui/kit"
)

func TestCellGlyphRejectsInvalidCellGeometry(t *testing.T) {
	for _, value := range []string{"界", "❤️", "ab", "\u0301", "\t", "\n", "\x1b[31m#", "\xff"} {
		t.Run(value, func(t *testing.T) {
			glyph, err := kit.NewCellGlyph(value)
			if !errors.Is(err, kit.ErrInvalidCellGlyph) {
				t.Fatalf("NewCellGlyph(%q) error = %v, want ErrInvalidCellGlyph", value, err)
			}
			if glyph != (kit.CellGlyph{}) {
				t.Fatal("rejected glyph returned usable display data")
			}
		})
	}
}

func TestCellGlyphKeepsItsCompleteGrapheme(t *testing.T) {
	for _, value := range []string{"", " ", "#", "e\u0301", "❤", "▏"} {
		glyph := newCellGlyph(t, value)
		if got := glyph.String(); got != value {
			t.Fatalf("glyph = %q, want %q", got, value)
		}
	}
	if got := (kit.CellGlyph{}).String(); got != "" {
		t.Fatalf("zero glyph = %q, want an omitted mark", got)
	}
}

func TestCellGlyphComposesAcrossCellBasedComponents(t *testing.T) {
	scenes := scenetest.New()
	t.Cleanup(scenes.Close)

	glyphs := kit.ASCII()
	accent := newCellGlyph(t, "e\u0301")
	glyphs.BarFull = accent
	glyphs.BarSteps = []kit.CellGlyph{newCellGlyph(t, "+")}
	progress := kit.Progress{Glyphs: glyphs, Done: 1, Total: 2}
	equalRows(t, paint(3, 1, progress.Draw), []string{"e\u0301+-"})
	chart := kit.BarChart{Glyphs: glyphs, Maximum: 2, Bars: []kit.Bar{{Value: 1}}}
	equalRows(t, paint(3, 1, chart.Draw), []string{"e\u0301+-"})
	glyphs.Sparkline = []kit.CellGlyph{newCellGlyph(t, "+"), accent}
	sparkline := kit.Sparkline{Glyphs: glyphs, Values: []float64{0, 1}}
	equalRows(t, paint(2, 1, sparkline.Draw), []string{"+e\u0301"})
	glyphs.SliderTrack, glyphs.SliderThumb = accent, newCellGlyph(t, "#")
	slider := kit.NewSlider(kit.SliderConfig{
		Behavior: base.SliderConfig{Initial: 1, Maximum: 2}, Glyphs: glyphs,
		Format: func(int) string { return "" },
	})
	equalRows(t, paintWidget(scenes, 5, 1, slider), []string{"e\u0301e\u0301#e\u0301e\u0301"})
	glyphs.ScrollTrack, glyphs.ScrollThumb = accent, newCellGlyph(t, "#")
	scrollbar := kit.Scrollbar{Glyphs: glyphs, Total: 6, Window: 2}
	equalRows(t, paint(1, 3, scrollbar.Draw), []string{"#", "e\u0301", "e\u0301"})
}

func TestOmittedCellGlyphDoesNotDisableOtherMarks(t *testing.T) {
	scenes := scenetest.New()
	t.Cleanup(scenes.Close)

	glyphs := kit.Glyphs{BarFull: newCellGlyph(t, "#")}
	progress := kit.Progress{Glyphs: glyphs, Done: 1, Total: 2}
	equalRows(t, paint(4, 1, progress.Draw), []string{"##.."})
	progress.Glyphs = kit.Glyphs{BarEmpty: newCellGlyph(t, "-")}
	equalRows(t, paint(4, 1, progress.Draw), []string{"..--"})
	chart := kit.BarChart{Glyphs: glyphs, Maximum: 2, Bars: []kit.Bar{{Value: 1}}}
	equalRows(t, paint(4, 1, chart.Draw), []string{"##.."})

	slider := kit.NewSlider(kit.SliderConfig{
		Behavior: base.SliderConfig{Initial: 1, Maximum: 2},
		Glyphs:   kit.Glyphs{SliderTrack: newCellGlyph(t, "-")},
		Format:   func(int) string { return "" },
	})
	equalRows(t, paintWidget(scenes, 5, 1, slider), []string{"--.--"})
	slider.SetGlyphs(kit.Glyphs{SliderThumb: newCellGlyph(t, "#")})
	equalRows(t, paintWidget(scenes, 5, 1, slider), []string{"..#.."})
	scrollbar := kit.Scrollbar{Glyphs: kit.Glyphs{ScrollTrack: newCellGlyph(t, "-")}, Total: 6, Window: 2}
	equalRows(t, paint(1, 3, scrollbar.Draw), []string{".", "-", "-"})
}

func TestDiffContinuationKeepsItsCellWithAnOmittedGlyph(t *testing.T) {
	for _, value := range []string{"", "e\u0301"} {
		glyph := newCellGlyph(t, value)
		content := kit.NewDiff(kit.DiffConfig{Glyphs: kit.Glyphs{Vertical: glyph}, Hunks: []diff.Hunk{{
			Lines: diff.Script{{Kind: diff.Added, Text: "abcdef"}},
		}}})
		mark := value
		if mark == "" {
			mark = " "
		}
		equalRows(t, paint(4, 2, content.Draw), []string{"+abc", mark + "def"})
	}
}

func newCellGlyph(t *testing.T, value string) kit.CellGlyph {
	t.Helper()
	glyph, err := kit.NewCellGlyph(value)
	if err != nil {
		t.Fatal(err)
	}
	return glyph
}
