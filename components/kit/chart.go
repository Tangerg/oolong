package kit

import (
	"image"
	"math"

	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/layout"
	"github.com/Tangerg/oolong/core/text"
)

// Sparkline is a one-row shape of the most recent numeric samples.
//
// It is passive presentation: Values remain caller-owned and may be replaced between
// frames. When more samples exist than columns, the newest ones are shown. Their
// finite minimum and maximum define the visible scale; a constant non-zero series is
// measured against zero so its magnitude remains visible. NaN and infinities leave
// their columns blank rather than inventing a height.
type Sparkline struct {
	Theme  Theme
	Glyphs Glyphs
	Values []float64
	// Minimum and Maximum fix the vertical domain when both are finite and
	// Maximum is greater than Minimum. Any other pair derives the domain from the
	// visible samples, which makes the zero value the automatic mode.
	Minimum, Maximum float64
}

// Measure is always one row. An empty stream leaves that row blank rather than
// moving the surrounding layout when its first sample arrives.
func (s Sparkline) Measure(int) int { return 1 }

// Draw paints the newest samples from left to right in the first row of v.
func (s Sparkline) Draw(v grid.View) {
	w, h := v.Size()
	if w <= 0 || h <= 0 || len(s.Values) == 0 || len(s.Glyphs.Sparkline) == 0 {
		return
	}
	values := s.Values[max(len(s.Values)-w, 0):]
	scale := (chartScale{minimum: s.Minimum, maximum: s.Maximum}).resolve(values)
	for x, value := range values {
		fraction, ok := scale.Fraction(value)
		if !ok {
			continue
		}
		level := int(math.Round(fraction * float64(len(s.Glyphs.Sparkline)-1)))
		v.Text(x, 0, s.Glyphs.Sparkline[level], s.Theme.Accent)
	}
}

// Bar is one category in a [BarChart]. Text is the caller's formatted value; an
// empty Text reserves no value column. A non-empty value is drawn in full or omitted
// when the chart is too narrow, never clipped into a different-looking value.
type Bar struct {
	Label string
	Value float64
	Text  string
}

// BarChart compares non-negative category values as horizontal bars.
//
// It is passive presentation, not a selection controller. Bars remain caller-owned.
// Maximum fixes the full extent when positive and finite; any other value derives it
// from the largest finite positive bar. Values at or below zero draw an empty track,
// values beyond the maximum fill it, and non-finite values draw empty rather than
// corrupting the scale.
type BarChart struct {
	Theme   Theme
	Glyphs  Glyphs
	Bars    []Bar
	Maximum float64
}

// Measure is one row per bar.
func (b BarChart) Measure(int) int { return len(b.Bars) }

// Draw paints the visible category rows of v with shared label, track, and value
// columns, so changing one value never moves another bar's geometry.
func (b BarChart) Draw(v grid.View) {
	w, h := v.Size()
	if w <= 0 || h <= 0 || len(b.Bars) == 0 {
		return
	}
	labelWidth, valueWidth := 0, 0
	for _, item := range b.Bars {
		labelWidth = max(labelWidth, text.Width(item.Label))
		valueWidth = max(valueWidth, text.Width(item.Text))
	}
	boxes := layoutMeter(w, labelWidth, valueWidth)
	scale := chartScale{maximum: b.maximum()}
	visible := v.Visible()
	first := min(max(visible.Min.Y, 0), len(b.Bars))
	last := min(max(visible.Max.Y, first), len(b.Bars))
	for row := first; row < last; row++ {
		item := b.Bars[row]
		fraction, _ := scale.Fraction(item.Value)
		offset := image.Pt(0, row)
		if item.Label != "" {
			Label{Text: item.Label, Style: b.Theme.Muted, Ellipsis: b.Glyphs.Ellipsis}.
				Draw(v.Sub(boxes.label.Add(offset)))
		}
		if item.Text != "" {
			Label{Text: item.Text, Style: b.Theme.Muted, Align: layout.End}.
				Draw(v.Sub(boxes.value.Add(offset)))
		}
		bar{
			fraction: fraction,
			glyphs:   b.Glyphs,
			full:     b.Theme.Accent,
			empty:    b.Theme.Subtle,
		}.Draw(v.Sub(boxes.track.Add(offset)))
	}
}

func (b BarChart) maximum() float64 {
	if finite(b.Maximum) && b.Maximum > 0 {
		return b.Maximum
	}
	maximum := 0.0
	for _, item := range b.Bars {
		if finite(item.Value) {
			maximum = max(maximum, item.Value)
		}
	}
	return max(maximum, 1)
}

// chartScale owns normalization for every chart shape. The rendered components ask
// it for fractions rather than duplicating NaN, infinity, constant-series, and clamp
// policy in each drawing loop.
type chartScale struct {
	minimum float64
	maximum float64
}

func (s chartScale) resolve(values []float64) chartScale {
	if s.valid() {
		return s
	}
	minimum, maximum := math.Inf(1), math.Inf(-1)
	for _, value := range values {
		if finite(value) {
			minimum, maximum = min(minimum, value), max(maximum, value)
		}
	}
	switch {
	case math.IsInf(minimum, 1):
		return chartScale{maximum: 1}
	case minimum < maximum:
		return chartScale{minimum: minimum, maximum: maximum}
	case minimum > 0:
		return chartScale{maximum: minimum}
	case minimum < 0:
		return chartScale{minimum: minimum}
	default:
		return chartScale{maximum: 1}
	}
}

func (s chartScale) Fraction(value float64) (float64, bool) {
	if !finite(value) || !s.valid() {
		return 0, false
	}
	return min(max((value-s.minimum)/(s.maximum-s.minimum), 0), 1), true
}

func (s chartScale) valid() bool {
	return finite(s.minimum) && finite(s.maximum) && s.maximum > s.minimum
}

func finite(value float64) bool { return !math.IsNaN(value) && !math.IsInf(value, 0) }
