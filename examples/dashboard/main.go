// Command dashboard is more panes than fit, with one of them showing.
//
// Two things worth looking at. The strip of names is drawn by the kit and the panes
// are held by the behaviour underneath it, so a press on a name selects a pane and
// everything else reaches the pane in the pane's own coordinates. And the table has
// a cursor: the rows and their order are one thing, where the columns go is another,
// and the two meet in four lines rather than in a widget that owns both.
// The activity pane projects those same tasks as progress, a bounded sparkline, and
// category bars; the application owns sampling and labels while the charts remain
// passive blocks.
//
// Alt+left and alt+right or 1–3 move between panes, the arrows move in them, a press
// on a heading sorts by it, and q leaves.
package main

import (
	"context"
	"fmt"
	"image"
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

// What a task can be doing. Named because the table sorts by them and the look
// colours them, and a string in three places is a string that will be misspelled in
// one of them.
const (
	waiting = "waiting"
	running = "running"
	done    = "done"
)

// task is a row of the table.
type task struct {
	name  string
	state string
	done  int
	total int
}

// dashboard is the strip and the panes under it.
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
	ratePreference   preference = "rate"
	motionPreference preference = "motion"
)

func newDashboard(runtime *program.Runtime) *dashboard {
	theme := kit.Suited(runtime.Environment().Ground())
	glyphs := kit.GlyphsFor(runtime.Environment().Locale())

	d := &dashboard{runtime: runtime, theme: theme, motion: true}
	d.work = newQueue(theme, glyphs)
	d.watch = newActivity(theme, glyphs, d.work)
	d.prefs = kit.NewSettings(kit.SettingsConfig[preference]{
		Theme: theme, Items: []preference{ratePreference, motionPreference},
		Label: func(item preference) string { return string(item) },
		Value: d.preferenceValue, Change: d.changePreference,
	})

	d.strip = kit.NewTabs(kit.TabsConfig{
		Theme: theme, Glyphs: glyphs,
		Items: []base.Tab{
			{Title: "tasks", Content: d.work.Element()},
			{Title: "activity", Content: d.watch.Element()},
			{Title: "settings", Content: d.prefs.Element()},
		},
	})
	d.tabs = d.strip.Controller()
	footer := scene.NewElement(scene.ElementConfig{Paint: func(frame scene.Frame) {
		kit.Label{Text: "1–3 or alt+←/→: pane   arrows: row or value   click a heading to sort   q: quit", Style: d.theme.Subtle}.Draw(frame.View)
	}})
	d.element = scene.NewGroup(scene.GroupConfig{Axis: layout.Down, Element: scene.ElementConfig{Key: "dashboard", Preview: func(event scene.Event) scene.EventResult { return scene.EventResult{Consumed: d.Handle(event.Input)} }}, Items: []scene.Item{
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
		switch key.Rune {
		case 'q':
			d.runtime.Quit()
			return true
		case '1', '2', '3':
			d.tabs.Select(int(key.Rune - '1'))
			if tab, ok := d.tabs.Current(); ok {
				_ = tab.Content.RequestFocusFirst()
			}
			return true
		}
	}
	return false
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

func (d *dashboard) changePreference(_ int, item preference, action keymap.Action) bool {
	switch item {
	case ratePreference:
		return d.watch.rate.Controller().Do(action)
	case motionPreference:
		switch action {
		case base.Decrease:
			d.motion = false
		case base.Increase:
			d.motion = true
		case base.Activate:
			d.motion = !d.motion
		default:
			return false
		}
		return true
	default:
		return false
	}
}

// queue is the tasks pane: rows with a cursor and an order, and the geometry they
// are drawn through.
type queue struct {
	element scene.Element
	theme   kit.Theme
	rows    *base.Table[task]
	view    kit.Table
	// columns are the complete table geometry from the last accepted frame. A press
	// arrives between frames and can only be about the boxes it saw.
	columns scene.Snapshot[kit.TableLayout]
}

func newQueue(theme kit.Theme, glyphs kit.Glyphs) *queue {
	q := &queue{theme: theme}
	q.rows = new(base.Table[task])
	q.rows.SetLess(func(a, b task, column int) bool {
		switch column {
		case 1:
			return a.state < b.state
		case 2:
			return a.done*b.total < b.done*a.total
		default:
			return a.name < b.name
		}
	})
	q.rows.SetItems(tasks())
	q.view = kit.Table{
		Theme:  theme,
		Glyphs: glyphs,
		Columns: []kit.Column{
			{Title: "task", Size: layout.Flex(2).AtLeast(8)},
			{Title: "state", Size: layout.Fixed(9)},
			{Title: "progress", Size: layout.Flex(3).AtLeast(12)},
		},
		Header: true,
		Sorted: q.rows.Sorted,
		Cell:   q.cell,
	}
	header := scene.NewElement(scene.ElementConfig{Paint: func(frame scene.Frame) {
		width, _ := frame.Size()
		columns := q.view.Layout(width)
		q.columns.Stage(frame, columns)
		columns.Titles(frame.View)
	}, Handle: func(event scene.Event) scene.EventResult {
		mouse, ok := event.Input.(input.Mouse)
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

func (q *queue) row(columns kit.TableLayout, v grid.View, at int, _ task, selected bool) {
	width, _ := v.Size()
	base := q.theme.Text
	if selected && q.rows.Focused() {
		base = base.Merge(q.theme.Selection)
		v.Fill(grid.Area(0, 0, width, 1), q.theme.Selection)
	}
	columns.Cells(v, at, base)
}

// cell draws one cell. The row index is the table's, so what is in a cell comes from
// this program's own rows and not from anything a widget copied.
func (q *queue) cell(row, column int) kit.Cell {
	item, ok := q.rows.At(row)
	if !ok {
		return kit.Cell{}
	}
	switch column {
	case 0:
		return kit.LabelCell(kit.Label{Text: item.name, Ellipsis: "…"})
	case 1:
		return kit.LabelCell(kit.Label{Text: item.state, Style: q.state(item), Ellipsis: "…"})
	default:
		return kit.Cell{
			Preferred: 12,
			Paint: func(v grid.View, _ grid.Style) {
				kit.Progress{
					Theme:   q.theme,
					Glyphs:  q.view.Glyphs,
					Done:    item.done,
					Total:   item.total,
					Percent: true,
				}.Draw(v)
			},
		}
	}
}

func (q *queue) state(of task) grid.Style {
	switch of.state {
	case done:
		return q.theme.Success
	case "failed":
		return q.theme.Danger
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
		item.state = running
		if item.done == item.total {
			item.state = done
		}
	}
	q.rows.SetItems(items)
}

func (q *queue) remaining() (finished, total int) {
	for _, item := range q.rows.Items() {
		finished += item.done
		total += item.total
	}
	return finished, total
}

// activity is the second pane: the queue as one number, and something that says work
// is happening at all.
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
		Theme: theme, Glyphs: glyphs, Initial: 1,
		Minimum: 1, Maximum: 4, Label: "rate",
	})
	activity.rate.Format = func(value int) string { return fmt.Sprintf("%d tasks/tick", value) }
	activity.spinner = kit.Spinner{
		Theme: theme, Glyphs: glyphs, Label: "watching",
	}
	activity.record()
	activity.element = scene.NewGroup(scene.GroupConfig{Element: scene.ElementConfig{Paint: activity.Draw}, Items: []scene.Item{{Element: activity.rate.Element()}}, Layout: func(space image.Point, _ []scene.Item) []image.Rectangle {
		return []image.Rectangle{grid.Area(0, 1, space.X, 1)}
	}}).Element()
	return activity
}

func (a *activity) tick() {
	a.spinner.Tick()
}

func (a *activity) Draw(v scene.Frame) {
	finished, total := a.of.remaining()
	// The rectangles are kept, not asked of the children afterwards: a child frame's
	// own bounds begin at zero, and staging a pointer region with those would put the
	// slider's row wherever the first row is.
	rects := (layout.Flow{Axis: layout.Down}).Rects(v.Bounds().Size(), []layout.Slot{
		{Size: layout.Fixed(1)},
		{Size: layout.Fixed(1)},
		{Size: layout.Fixed(1)},
		{Size: layout.Fixed(1)},
		{Size: layout.Flex(1).AtLeast(a.of.rows.Len())},
	})
	rows := v.Subs(rects)
	kit.Progress{
		Theme:   a.theme,
		Glyphs:  a.glyphs,
		Label:   "all of it",
		Done:    finished,
		Total:   total,
		Percent: true,
	}.Draw(rows[0].View)
	kit.Sparkline{
		Theme: a.theme, Glyphs: a.glyphs, Values: a.samples,
		Minimum: 0, Maximum: 1,
	}.Draw(rows[2].View)

	// A spinner is for work with no total and a bar is for work with one. Both are
	// here because the two questions are different: how far along, and is anything
	// happening at all.
	if finished >= total {
		kit.Label{Text: "everything is done", Style: a.theme.Success}.Draw(rows[3].View)
	} else {
		a.spinner.Draw(rows[3].View)
	}
	kit.BarChart{
		Theme: a.theme, Glyphs: a.glyphs, Maximum: 100,
		Bars: a.bars(),
	}.Draw(rows[4].View)
}

func (a *activity) Element() scene.Element { return a.element }

// record keeps a bounded history of aggregate completion. The dashboard owns this
// sampling policy; Sparkline only projects the values it is given.
func (a *activity) record() {
	finished, total := a.of.remaining()
	value := 0.0
	if total > 0 {
		value = float64(finished) / float64(total)
	}
	if len(a.samples) == cap(a.samples) {
		copy(a.samples, a.samples[1:])
		a.samples[len(a.samples)-1] = value
		return
	}
	a.samples = append(a.samples, value)
}

// bars projects the task model into chart categories. Labels, percentage formatting,
// and the decision to compare completion are application grammar, not kit policy.
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
		{name: "build", state: waiting, done: 3, total: 12},
		{name: "test", state: waiting, done: 1, total: 20},
		{name: "vet", state: waiting, done: 0, total: 6},
		{name: "lint", state: waiting, done: 0, total: 9},
	}
}
