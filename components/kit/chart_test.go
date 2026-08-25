package kit_test

import (
	"math"
	"testing"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
)

func TestSparklineDrawsTheNewestVisibleSamples(t *testing.T) {
	chart := kit.Sparkline{
		Glyphs: kit.Unicode(),
		Values: []float64{-10, -5, 0, 1, 2, 3},
	}
	if got := chart.Measure(4); got != 1 {
		t.Fatalf("Measure = %d, want one row", got)
	}
	equalRows(t, paint(4, 1, chart.Draw), []string{"▁▃▆█"})
}

func TestSparklineCanFixItsDomain(t *testing.T) {
	barelyStarted := kit.Sparkline{
		Glyphs: kit.Unicode(), Minimum: 0, Maximum: 1,
		Values: []float64{0, 0.005, 0.01, 0.015, 0.02},
	}
	nearlyDone := kit.Sparkline{
		Glyphs: kit.Unicode(), Minimum: 0, Maximum: 1,
		Values: []float64{0.96, 0.965, 0.97, 0.975, 0.98},
	}
	equalRows(t, paint(5, 1, barelyStarted.Draw), []string{"▁▁▁▁▁"})
	equalRows(t, paint(5, 1, nearlyDone.Draw), []string{"█████"})
}

func TestSparklineDerivesItsDomainWhenBoundsAreInvalid(t *testing.T) {
	chart := kit.Sparkline{
		Glyphs: kit.Unicode(), Minimum: math.NaN(), Maximum: 10,
		Values: []float64{4, 6},
	}
	equalRows(t, paint(2, 1, chart.Draw), []string{"▁█"})
}

func TestSparklineKeepsItsRowBeforeTheFirstSample(t *testing.T) {
	if got := (kit.Sparkline{}).Measure(80); got != 1 {
		t.Fatalf("Measure = %d, want one stable row", got)
	}
}

func TestSparklineKeepsInvalidSamplesOutOfItsScale(t *testing.T) {
	chart := kit.Sparkline{
		Glyphs: kit.Unicode(),
		Values: []float64{math.NaN(), 0, math.Inf(1), 1},
	}
	equalRows(t, paint(4, 1, chart.Draw), []string{".▁.█"})

	constant := kit.Sparkline{Glyphs: kit.ASCII(), Values: []float64{5, 5}}
	equalRows(t, paint(2, 1, constant.Draw), []string{"@@"})
}

func TestBarChartSharesStableColumnsAcrossRows(t *testing.T) {
	chart := kit.BarChart{
		Glyphs: kit.ASCII(),
		Bars: []kit.Bar{
			{Label: "one", Value: 5, Text: "5"},
			{Label: "ten", Value: 10, Text: "10"},
		},
	}
	if got := chart.Measure(20); got != 2 {
		t.Fatalf("Measure = %d, want two rows", got)
	}
	equalRows(t, paint(20, 2, chart.Draw), []string{
		"one.######-------..5",
		"ten.#############.10",
	})
}

func TestBarChartMaximumAndInvalidValuesHaveDefinedGeometry(t *testing.T) {
	chart := kit.BarChart{
		Glyphs:  kit.ASCII(),
		Maximum: 20,
		Bars: []kit.Bar{
			{Label: "half", Value: 10},
			{Label: "bad", Value: math.NaN()},
		},
	}
	equalRows(t, paint(12, 2, chart.Draw), []string{
		"half.###----",
		"bad..-------",
	})
}

func TestBarChartNeverClipsAValueIntoAnotherNumber(t *testing.T) {
	chart := kit.BarChart{
		Glyphs: kit.ASCII(), Maximum: 100,
		Bars: []kit.Bar{{Label: "q", Value: 5, Text: "7000%"}},
	}
	equalRows(t, paint(3, 1, chart.Draw), []string{"q.-"})
	equalRows(t, paint(5, 1, chart.Draw), []string{"7000%"})
}

var (
	_ headless.Block = kit.Sparkline{}
	_ headless.Block = kit.BarChart{}
)
