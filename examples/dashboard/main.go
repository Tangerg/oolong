// Command dashboard is more panes than fit, with one of them showing.
//
// Two things worth looking at. The strip of names is drawn by the kit and the panes
// are held by the behaviour underneath it, so a press on a name selects a pane and
// everything else reaches the pane in the pane's own coordinates. And the table has
// a cursor: the rows and their order are one thing, where the columns go is another,
// and the two meet in four lines rather than in a widget that owns both.
//
// Alt+left and alt+right or 1–3 move between panes, the arrows move in them, a press
// on a heading sorts by it, and q leaves.
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/keymap"
	"github.com/Tangerg/oolong/core/layout"
	"github.com/Tangerg/oolong/core/program"
	"github.com/Tangerg/oolong/core/term"
)

func main() {
	if err := program.Run(context.Background(), program.Config{
		Root: func(runtime *program.Runtime) program.Component {
			return headless.NewRoot(newDashboard(runtime))
		},
		Terminal: term.Features{Probe: true, Mouse: true},
	}); err != nil {
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

	pane   int
	tabs   *headless.Tabs
	strip  kit.Tabs
	work   *queue
	watch  *activity
	prefs  *kit.Settings[preference]
	motion bool
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

	d.strip = *kit.NewTabs(kit.TabsConfig{
		Theme: theme, Glyphs: glyphs, Selection: headless.Bind(&d.pane),
		Items: []headless.Tab{
			{Title: "tasks", Of: d.work},
			{Title: "activity", Of: d.watch},
			{Title: "settings", Of: d.prefs},
		},
	})
	d.tabs = d.strip.Controller()
	d.tabs.Focus(true)

	// Something to watch: the work advances on a clock this program started, and an
	// interface with nothing animating asks for no frames at all.
	runtime.Every(120*time.Millisecond, d.advance)
	return d
}

// Draw is the strip, whichever pane it says, and a hint row under both.
func (d *dashboard) Draw(v headless.Frame) {
	rows := v.Subs((layout.Flow{Axis: layout.Down}).Rects(v.Bounds().Size(), []layout.Slot{
		{Size: layout.Flex(1)},
		{Size: layout.Fixed(1)},
	}))
	d.strip.Draw(rows[0])
	kit.Label{
		Text:  "1–3 or alt+←/→: pane   arrows: row or value   click a heading to sort   q: quit",
		Style: d.theme.Subtle,
	}.Draw(rows[1].View)
}

func (d *dashboard) Handle(ev input.Event) bool {
	if key, ok := ev.(input.Key); ok && key.Down() {
		switch key.Rune {
		case 'q':
			d.runtime.Quit()
			return true
		case '1', '2', '3':
			// The shortcut edits application state; Sync performs the controller's
			// focus transition without turning Draw into a semantic state change.
			d.pane = int(key.Rune - '1')
			d.tabs.Sync()
			return true
		}
	}
	return d.strip.Handle(ev)
}

func (d *dashboard) advance() {
	d.work.advance(d.watch.rateValue)
	if d.motion {
		d.watch.tick()
	}
}

func (d *dashboard) preferenceValue(item preference) string {
	switch item {
	case ratePreference:
		return fmt.Sprintf("%d tasks/tick", d.watch.rateValue)
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
		case headless.Decrease:
			d.motion = false
		case headless.Increase:
			d.motion = true
		case headless.Activate:
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
	theme kit.Theme
	rows  *headless.Table[task]
	view  kit.Table
	// columns are the complete table geometry from the last accepted frame. A press
	// arrives between frames and can only be about the boxes it saw.
	columns headless.Snapshot[kit.TableLayout]
}

func newQueue(theme kit.Theme, glyphs kit.Glyphs) *queue {
	q := &queue{theme: theme}
	q.rows = new(headless.Table[task])
	// One comparison for every column, which is what a table sorted by a column
	// somebody named needs.
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
		// The header marks the column the rows are in the order of, which is the only
		// way for a reader to tell an order from a coincidence. It is the table's own
		// answer, wired straight in.
		Sorted: q.rows.Sorted,
		Cell:   q.cell,
	}
	return q
}

// Draw is the header, then the rows.
//
// The header is the kit's and the rows are the behaviour's, and they agree about
// where the columns are because they ask the same table. A widget that owned both
// would own the cursor and the scrolling as well.
func (q *queue) Draw(v headless.Frame) {
	width, _ := v.Size()
	columns := q.view.Layout(width)
	q.columns.Stage(v, columns)
	bands := v.Subs((layout.Flow{Axis: layout.Down}).Rects(v.Bounds().Size(), []layout.Slot{
		{Size: layout.Fixed(1)},
		{Size: layout.Flex(1)},
	}))
	columns.Titles(bands[0].View)
	q.rows.DrawRows(bands[1], func(v grid.View, at int, item task, selected bool) {
		q.row(columns, v, at, item, selected)
	})
}

func (q *queue) Measure(int) int { return q.rows.Len() + 1 }

// Handle sorts by a heading that was pressed, and hands everything else to the rows
// — one row down, because the header took the first one.
func (q *queue) Handle(ev input.Event) bool {
	if mouse, ok := ev.(input.Mouse); ok {
		if mouse.Pos.Y == 0 {
			if mouse.Action != input.MouseDown {
				return false
			}
			column, on := q.columns.Value().ColumnAt(mouse.Pos.X)
			return on && q.rows.SortBy(column)
		}
		mouse.Pos.Y--
		return q.rows.Handle(mouse)
	}
	return q.rows.Handle(ev)
}

func (q *queue) Focus(has bool) { q.rows.Focus(has) }

// row draws the band a row sits in, and then its cells through the geometry the
// header used.
func (q *queue) row(columns kit.TableLayout, v grid.View, at int, _ task, selected bool) {
	width, _ := v.Size()
	base := q.theme.Text
	if selected && q.rows.Focused() {
		base = base.Merge(q.theme.Selection)
		v.Fill(grid.Rect(0, 0, width, 1), q.theme.Selection)
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

// advance moves every task along by one step.
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

// remaining is how much of the whole queue is left, which is what the other pane is
// about.
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
	// rateValue belongs to the dashboard rather than its appearance. The slider and
	// settings pane both edit this one value, so neither can drift from the other.
	rateValue int
	rate      *kit.Slider
}

func newActivity(theme kit.Theme, glyphs kit.Glyphs, of *queue) *activity {
	activity := &activity{theme: theme, glyphs: glyphs, of: of, rateValue: 1}
	activity.rate = kit.NewSlider(kit.SliderConfig{
		Theme: theme, Glyphs: glyphs, Value: headless.Bind(&activity.rateValue),
		Minimum: 1, Maximum: 4, Label: "rate",
	})
	activity.rate.Format = func(value int) string { return fmt.Sprintf("%d tasks/tick", value) }
	activity.spinner = kit.Spinner{
		Theme: theme, Glyphs: glyphs, Label: "watching",
	}
	return activity
}

func (a *activity) tick() { a.spinner.Tick() }

func (a *activity) Measure(int) int { return 4 }

func (a *activity) Draw(v headless.Frame) {
	finished, total := a.of.remaining()
	rows := v.Subs((layout.Flow{Axis: layout.Down}).Rects(v.Bounds().Size(), []layout.Slot{
		{Size: layout.Fixed(1)},
		{Size: layout.Fixed(1)},
		{Size: layout.Fixed(1)},
		{Size: layout.Flex(1)},
	}))
	kit.Progress{
		Theme:   a.theme,
		Glyphs:  a.glyphs,
		Label:   "all of it",
		Done:    finished,
		Total:   total,
		Percent: true,
	}.Draw(rows[0].View)
	a.rate.Draw(rows[1])

	// A spinner is for work with no total and a bar is for work with one. Both are
	// here because the two questions are different: how far along, and is anything
	// happening at all.
	if finished >= total {
		kit.Label{Text: "everything is done", Style: a.theme.Success}.Draw(rows[3].View)
		return
	}
	a.spinner.Draw(rows[3].View)
}

func (a *activity) Handle(event input.Event) bool { return a.rate.Handle(event) }

func (a *activity) Focus(has bool) { a.rate.Focus(has) }

func tasks() []task {
	return []task{
		{name: "build", state: waiting, done: 3, total: 12},
		{name: "test", state: waiting, done: 1, total: 20},
		{name: "vet", state: waiting, done: 0, total: 6},
		{name: "lint", state: waiting, done: 0, total: 9},
	}
}
