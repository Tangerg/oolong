// Command dashboard keeps task data with the application while scene children own
// input and layout, so multiple presentations share one task model.
package main

import (
	"cmp"
	"context"
	"fmt"
	"os"
	"time"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/keymap"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/program"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/core/term"
	"github.com/Tangerg/oolong/ui/kit"
)

func main() {
	app, err := program.NewScreen(program.ScreenConfig{Terminal: term.Features{Probe: true, Mouse: true}, Build: func(runtime *program.Runtime) (scene.Element, error) {
		return newDashboard(runtime).Element(), nil
	}})
	if err == nil {
		err = app.Run(context.Background())
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "dashboard:", err)
		os.Exit(1)
	}
}

const (
	waiting = "waiting"
	running = "running"
	done    = "done"
)

type task struct {
	name  string
	done  int
	total int
}

func (t task) state() string {
	switch {
	case t.done >= t.total:
		return done
	case t.done > 0:
		return running
	default:
		return waiting
	}
}

type dashboard struct {
	runtime *program.Runtime
	theme   kit.Theme

	element scene.Element
	timer   *program.Timer
	scope   *program.Scope
	tabs    *base.Tabs
	strip   *kit.Tabs
	work    *queue
	watch   *activity
	prefs   *kit.Settings[preference]
	motion  bool
}

type preference string

const (
	ratePreference    preference    = "rate"
	motionPreference  preference    = "motion"
	quitDashboard     keymap.Action = "quit"
	firstPaneShortcut               = '1'
	lastPaneShortcut                = '9'
)

func newDashboard(runtime *program.Runtime) *dashboard {
	theme := kit.Suited(runtime.Environment().Ground())
	glyphs := kit.GlyphsFor(runtime.Environment().Locale())

	d := &dashboard{runtime: runtime, theme: theme, motion: true}
	keys := base.DefaultTabsKeys()
	keys.Bind(quitDashboard, input.Chord{Rune: 'q'})
	d.work = newQueue(theme, glyphs)
	d.watch = newActivity(theme, glyphs, d.work)
	d.prefs = kit.NewSettings(kit.SettingsConfig[preference]{
		Theme: theme, Items: []preference{ratePreference, motionPreference},
		Label: func(item preference) string { return string(item) },
		Value: d.preferenceValue, Change: d.changePreference,
	})

	d.strip = kit.NewTabs(kit.TabsConfig{
		Theme: theme, Glyphs: glyphs, Keys: keys,
		Items: []base.Tab{
			{Title: "tasks", Content: d.work.Element()},
			{Title: "activity", Content: d.watch.Element()},
			{Title: "settings", Content: d.prefs.Element()},
		},
	})
	d.tabs = d.strip.Controller()
	footer := scene.NewElement(scene.ElementConfig{Paint: d.drawHelp})
	d.element = scene.NewGroup(scene.GroupConfig{Axis: layout.Down, Element: scene.ElementConfig{Key: "dashboard", Preview: func(event scene.Event) scene.EventResult { return scene.EventResult{Consumed: d.Handle(event.Input())} }}, Items: []scene.Item{
		{Element: d.strip.Element(), Size: layout.Flex(1)}, {Element: footer, Size: layout.Fixed(1)},
	}}).Element()
	var err error
	d.scope, err = runtime.NewScope(program.ScopeConfig{})
	if err != nil {
		panic(err)
	}
	d.timer, err = runtime.Every(d.scope, 120*time.Millisecond, d.advance)
	if err != nil {
		d.scope.Close()
		panic(err)
	}
	return d
}

func (d *dashboard) Element() scene.Element { return d.element }

func (d *dashboard) Handle(ev input.Event) bool {
	if key, ok := ev.(input.Key); ok && key.Down() {
		if action, ok := d.tabs.Keys.ActionFor(key); ok && action == quitDashboard {
			d.runtime.Quit()
			return true
		}
		first, last := d.paneShortcutRange()
		if first == 0 {
			return false
		}
		for chord := range key.ShortcutChords() {
			if chord.Code != input.Character || chord.Mods != 0 || chord.Rune < first || chord.Rune > last {
				continue
			}
			d.tabs.Select(int(chord.Rune - first))
			if tab, ok := d.tabs.Current(); ok {
				_ = tab.Content.RequestFocusFirst()
			}
			return true
		}
	}
	return false
}

func (d *dashboard) paneShortcutRange() (first, last rune) {
	count := d.tabs.Len()
	if count <= 0 {
		return 0, 0
	}
	if count >= lastPaneShortcut-firstPaneShortcut+1 {
		return firstPaneShortcut, lastPaneShortcut
	}
	return firstPaneShortcut, firstPaneShortcut + rune(count) - 1
}

func (d *dashboard) drawHelp(frame scene.Frame) {
	width, height := frame.Size()
	var prefix string
	first, last := d.paneShortcutRange()
	if first != 0 && first == last {
		prefix = fmt.Sprintf("%c pane  ", first)
	} else if first != 0 {
		prefix = fmt.Sprintf("%c–%c pane  ", first, last)
	}
	x := frame.Text(0, 0, prefix, d.theme.Subtle)
	kit.Help{Theme: d.theme, Keys: d.tabs.Keys, Show: []keymap.Action{base.PrevTab, base.NextTab, quitDashboard}}.
		Draw(frame.View.Sub(grid.Area(x, 0, width-x, height)))
}

func (d *dashboard) advance() {
	d.work.advance(d.watch.rate.Controller().Value())
	d.watch.record()
	if d.motion {
		d.watch.tick()
	}
}

func (d *dashboard) preferenceValue(item preference) string {
	switch item {
	case ratePreference:
		return fmt.Sprintf("%d tasks/tick", d.watch.rate.Controller().Value())
	case motionPreference:
		if d.motion {
			return "on"
		}
		return "off"
	default:
		return ""
	}
}

func (d *dashboard) changePreference(_ int, item preference, action keymap.Action) scene.ActionResult[bool] {
	result := scene.ActionResult[bool]{}
	switch item {
	case ratePreference:
		rate := d.watch.rate.Controller()
		previous := rate.Value()
		result.Value = rate.Do(action)
		if rate.Value() != previous {
			result.State = scene.Applied
		}
	case motionPreference:
		previous := d.motion
		switch action {
		case base.Decrease:
			d.motion = false
		case base.Increase:
			d.motion = true
		case base.Activate:
			d.motion = !d.motion
		default:
			return scene.ActionResult[bool]{}
		}
		result.Value = true
		if d.motion != previous {
			result.State = scene.Applied
		}
	}
	return result
}

type taskColumn struct {
	header  kit.Column
	compare func(task, task) int
	cell    func(task) kit.Cell
}

type queue struct {
	element     scene.Element
	theme       kit.Theme
	rows        *base.Table[task]
	view        kit.Table
	definitions []taskColumn
	// columns are the complete table geometry from the last accepted frame. A press
	// arrives between frames and can only be about the boxes it saw.
	columns scene.Snapshot[kit.TableLayout]
}

func newQueue(theme kit.Theme, glyphs kit.Glyphs) *queue {
	q := &queue{theme: theme}
	q.rows = new(base.Table[task])
	q.definitions = q.taskColumns()
	q.rows.SetSortColumns(q.sortColumn)
	q.rows.SetItems(tasks())
	headers := make([]kit.Column, len(q.definitions))
	for i, column := range q.definitions {
		headers[i] = column.header
	}
	q.view = kit.Table{
		Theme:   theme,
		Glyphs:  glyphs,
		Columns: headers,
		Header:  true,
		Sorted:  q.rows.Sorted,
		Cell:    q.cell,
	}
	header := scene.NewElement(scene.ElementConfig{Paint: func(frame scene.Frame) {
		width, _ := frame.Size()
		columns := q.view.Layout(width)
		q.columns.Stage(frame, columns)
		columns.Titles(frame.View)
	}, Handle: func(event scene.Event) scene.EventResult {
		mouse, ok := event.Input().(input.Mouse)
		if !ok || mouse.Action != input.MouseDown || mouse.Button != input.ButtonLeft {
			return scene.EventResult{}
		}
		columns, known := event.Read(&q.columns)
		if !known {
			return scene.EventResult{}
		}
		column, on := columns.ColumnAt(mouse.Pos.X)
		return scene.EventResult{Consumed: on && q.rows.SortBy(column)}
	}})
	if err := q.rows.Element().SetPaint(func(frame scene.Frame) {
		width, _ := frame.Size()
		columns := q.view.Layout(width)
		q.rows.DrawRows(frame, func(view grid.View, at int, item task, selected bool) { q.row(columns, view, at, item, selected) })
	}); err != nil {
		panic(err)
	}
	q.element = scene.NewGroup(scene.GroupConfig{Axis: layout.Down, Items: []scene.Item{{Element: header, Size: layout.Fixed(1)}, {Element: q.rows.Element(), Size: layout.Flex(1)}}}).Element()
	return q
}

func (q *queue) Element() scene.Element { return q.element }

const progressCellWidth = 12

func (q *queue) taskColumns() []taskColumn {
	return []taskColumn{
		{
			header:  kit.Column{Title: "task", Size: layout.Flex(2).AtLeast(8)},
			compare: func(a, b task) int { return cmp.Compare(a.name, b.name) },
			cell: func(item task) kit.Cell {
				return kit.LabelCell(kit.Label{Text: item.name, Ellipsis: q.view.Glyphs.Ellipsis})
			},
		},
		{
			header:  kit.Column{Title: "state", Size: layout.Fixed(9)},
			compare: func(a, b task) int { return cmp.Compare(a.state(), b.state()) },
			cell: func(item task) kit.Cell {
				return kit.LabelCell(kit.Label{Text: item.state(), Style: q.stateStyle(item), Ellipsis: q.view.Glyphs.Ellipsis})
			},
		},
		{
			header:  kit.Column{Title: "progress", Size: layout.Flex(3).AtLeast(progressCellWidth)},
			compare: func(a, b task) int { return cmp.Compare(a.done*b.total, b.done*a.total) },
			cell:    q.progressCell,
		},
	}
}

func (q *queue) sortColumn(column int) func(task, task) int {
	if column < 0 || column >= len(q.definitions) {
		return nil
	}
	return q.definitions[column].compare
}

func (q *queue) row(columns kit.TableLayout, v grid.View, at int, _ task, selected bool) {
	width, _ := v.Size()
	base := q.theme.Text
	if selected && q.rows.Focused() {
		base = base.Merge(q.theme.Selection)
		v.Fill(grid.Area(0, 0, width, 1), q.theme.Selection)
	}
	columns.Cells(v, at, base)
}

// The table owns row order, while application rows own cell content; rendering
// reads that mapping without retaining a second task model.
func (q *queue) cell(row, column int) kit.Cell {
	item, ok := q.rows.At(row)
	if !ok || column < 0 || column >= len(q.definitions) {
		return kit.Cell{}
	}
	return q.definitions[column].cell(item)
}

func (q *queue) progressCell(item task) kit.Cell {
	return kit.Cell{
		Preferred: progressCellWidth,
		Paint: func(v grid.View, _ grid.Style) {
			kit.Progress{
				Theme: q.theme, Glyphs: q.view.Glyphs,
				Done: item.done, Total: item.total, Percent: true,
			}.Draw(v)
		},
	}
}

func (q *queue) stateStyle(of task) grid.Style {
	switch of.state() {
	case done:
		return q.theme.Success
	default:
		return q.theme.Muted
	}
}

func (q *queue) advance(by int) {
	items := q.rows.Items()
	for i := range items {
		item := &items[i]
		if item.done >= item.total {
			continue
		}
		item.done = min(item.done+max(by, 0), item.total)
	}
	q.rows.SetItems(items)
}

func (q *queue) completionTotals() (completedUnits, totalUnits int) {
	for _, item := range q.rows.Items() {
		completedUnits += item.done
		totalUnits += item.total
	}
	return completedUnits, totalUnits
}

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
		samples: make([]float64, 0, 64),
	}
	activity.rate = kit.NewSlider(kit.SliderConfig{
		Behavior: base.SliderConfig{
			Initial: 1,
			Minimum: 1, Maximum: 4, Label: "rate",
		}, Theme: theme, Glyphs: glyphs,
	})
	activity.rate.Format = func(value int) string { return fmt.Sprintf("%d tasks/tick", value) }
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

func tasks() []task {
	return []task{
		{name: "build", done: 3, total: 12},
		{name: "test", done: 1, total: 20},
		{name: "vet", done: 0, total: 6},
		{name: "lint", done: 0, total: 9},
	}
}
