package kit_test

import (
	"errors"
	"math"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/grid"
)

func TestChartExtremaRemainFiniteAndFractionalBarsUseTheirDomain(t *testing.T) {
	for _, values := range [][]float64{{-math.MaxFloat64, 0, math.MaxFloat64}, {math.SmallestNonzeroFloat64, 2 * math.SmallestNonzeroFloat64}} {
		spark := kit.Sparkline{Glyphs: kit.ASCII(), Values: values}
		grid.Render(3, 1, spark.Draw)
	}
	chart := kit.BarChart{Glyphs: kit.ASCII(), Bars: []kit.Bar{{Value: 0.1}}}
	got := grid.Render(10, 1, chart.Draw)[0]
	if len(strings.TrimSpace(got)) < 5 {
		t.Fatalf("fractional maximum did not fill track: %q", got)
	}
}

func TestASCIITruncationUsesConfiguredGlyphs(t *testing.T) {
	box := kit.Box{Glyphs: kit.ASCII(), Title: "a long ascii title"}
	for _, row := range grid.Render(10, 3, func(v grid.View) { box.Draw(v) }) {
		if utf8.RuneCountInString(row) != len(row) {
			t.Fatalf("non-ASCII truncation: %q", row)
		}
	}
}

func TestFormDisplaysCrossFieldValidationError(t *testing.T) {
	controller := headless.NewForm(&headless.Text{})
	controller.Check = func() error { return errors.New("answers conflict") }
	form := kit.NewForm(kit.FormConfig{Controller: controller})
	before := form.HeightForWidth(30)
	controller.Submit()
	if form.HeightForWidth(30) != before+1 {
		t.Fatal("form error has no layout row")
	}
	root := headless.NewRoot(form)
	rows := grid.Render(30, form.HeightForWidth(30), root.Draw)
	if !strings.Contains(strings.Join(rows, "\n"), "answers conflict") {
		t.Fatalf("error not drawn: %q", rows)
	}
}
