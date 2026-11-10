package main

import (
	"fmt"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/kit"
)

type activity struct {
	theme   kit.Theme
	glyphs  kit.Glyphs
	of      *queue
	spinner kit.Spinner
	samples []float64
	rate    *kit.Slider
	element scene.Element
}

func newActivity(theme kit.Theme, glyphs kit.Glyphs, of *queue) *activity {
	activity := &activity{
		theme: theme, glyphs: glyphs, of: of,
		samples: make([]float64, 0, activityHistoryLength),
	}
	activity.rate = kit.NewSlider(kit.SliderConfig{
		Behavior: base.SliderConfig{
			Initial: 1,
			Minimum: 1, Maximum: maximumUnitsPerTick, Label: "rate",
		}, Theme: theme, Glyphs: glyphs,
	})
	activity.rate.SetFormat(formatRate)
	activity.spinner = kit.Spinner{
		Theme: theme, Glyphs: glyphs, Label: "watching",
	}
	activity.record()
	activity.element = scene.NewGroup(scene.GroupConfig{Axis: layout.Down, Items: []scene.Item{
		{Element: scene.NewElement(scene.ElementConfig{Paint: activity.drawProgress}), Size: layout.Fixed(1)},
		{Element: activity.rate.Element(), Size: layout.Fixed(1)},
		{Element: scene.NewElement(scene.ElementConfig{Paint: activity.drawHistory}), Size: layout.Fixed(1)},
		{Element: scene.NewElement(scene.ElementConfig{Paint: activity.drawStatus}), Size: layout.Fixed(1)},
		{Element: scene.NewElement(scene.ElementConfig{Paint: activity.drawTasks}), Size: layout.Flex(1)},
	}}).Element()
	return activity
}

func (a *activity) tick() {
	a.spinner.Tick()
}

func (a *activity) drawProgress(frame scene.Frame) {
	completedUnits, totalUnits := a.of.completionTotals()
	kit.Progress{
		Theme:   a.theme,
		Glyphs:  a.glyphs,
		Label:   "all of it",
		Done:    completedUnits,
		Total:   totalUnits,
		Percent: true,
	}.Draw(frame.View)
}

func (a *activity) drawHistory(frame scene.Frame) {
	kit.Sparkline{
		Theme: a.theme, Glyphs: a.glyphs, Values: a.samples,
		Minimum: 0, Maximum: 1,
	}.Draw(frame.View)
}

func (a *activity) drawStatus(frame scene.Frame) {
	completedUnits, totalUnits := a.of.completionTotals()
	if completedUnits >= totalUnits {
		kit.Label{Text: "everything is done", Style: a.theme.Success}.Draw(frame.View)
	} else {
		a.spinner.Draw(frame.View)
	}
}

func (a *activity) drawTasks(frame scene.Frame) {
	kit.BarChart{
		Theme: a.theme, Glyphs: a.glyphs, Maximum: 100,
		Bars: a.bars(),
	}.Draw(frame.View)
}

func (a *activity) Element() scene.Element { return a.element }

// The dashboard owns sampling policy; Sparkline only projects its bounded history.
func (a *activity) record() {
	completedUnits, totalUnits := a.of.completionTotals()
	value := 0.0
	if totalUnits > 0 {
		value = float64(completedUnits) / float64(totalUnits)
	}
	if len(a.samples) == cap(a.samples) {
		copy(a.samples, a.samples[1:])
		a.samples[len(a.samples)-1] = value
		return
	}
	a.samples = append(a.samples, value)
}

// Category labels and completion comparison are application grammar, so chart
// blocks receive a projection of the task model.
func (a *activity) bars() []kit.Bar {
	items := a.of.rows.Items()
	bars := make([]kit.Bar, len(items))
	for i, item := range items {
		percent := 0.0
		if item.total > 0 {
			percent = float64(item.done) / float64(item.total) * 100
		}
		bars[i] = kit.Bar{
			Label: item.name,
			Value: percent,
			Text:  fmt.Sprintf("%.0f%%", percent),
		}
	}
	return bars
}
