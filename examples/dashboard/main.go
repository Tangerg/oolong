// Command dashboard is more panes than fit, with one of them showing.
//
// Two things worth looking at. The strip of names is drawn by the kit and the panes
// are held by the behaviour underneath it, so a press on a name selects a pane and
// everything else reaches the pane in the pane's own coordinates. And the table has
// a cursor: the rows and their order are one thing, where the columns go is another,
// and the two meet in four lines rather than in a widget that owns both.
//
// Alt+left and alt+right move between panes, the arrows move in them, a press on a
// heading sorts by it, and q leaves.
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
	"github.com/Tangerg/oolong/core/layout"
	"github.com/Tangerg/oolong/core/program"
	"github.com/Tangerg/oolong/core/term"
)

func main() {
	if err := program.Run(context.Background(), program.Config{
		Root: func(runtime *program.Runtime) program.Component {
			return headless.NewRoot(newDashboard(runtime))
		},
		Terminal: term.Options{Probe: true, Mouse: true},
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

	tabs  *headless.Tabs
	strip kit.Tabs
	work  *queue
	watch *activity
}

func newDashboard(runtime *program.Runtime) *dashboard {
	theme := kit.Suited(runtime.Environment().Ground())
	glyphs := kit.GlyphsFor(os.Getenv)

	d := &dashboard{runtime: runtime, theme: theme}
	d.work = newQueue(theme, glyphs)
	d.watch = newActivity(theme, glyphs, d.work)

	d.strip = *kit.NewTabs(
		theme,
		glyphs,
		headless.Tab{Title: "tasks", Of: d.work},
		headless.Tab{Title: "activity", Of: d.watch},
	)
	d.tabs = d.strip.Of
	d.tabs.Focus(true)

	// Something to watch: the work advances on a clock this program started, and an
	// interface with nothing animating asks for no frames at all.
	runtime.Every(120*time.Millisecond, d.advance)
	return d
}

// Draw is the strip, whichever pane it says, and a hint row under both.
func (d *dashboard) Draw(v headless.Frame) {
	rows := v.Subs(layout.Down.Rects(v.Bounds().Size(),
		layout.Slot{Size: layout.Flex(1)},
		layout.Slot{Size: layout.Fixed(1)},
	))
	d.strip.Draw(rows[0])
	kit.Label{
		Text:  "alt+←/→: pane   arrows: row or rate   click a heading to sort   q: quit",
		Style: d.theme.Subtle,
	}.Draw(rows[1].View)
}

func (d *dashboard) Handle(ev input.Event) bool {
	if key, ok := ev.(input.Key); ok && key.Down() && key.Rune == 'q' {
		d.runtime.Quit()
		return true
	}
	return d.strip.Handle(ev)
}

func (d *dashboard) advance() {
	d.work.advance(d.watch.rate.Of.Value())
	d.watch.tick()
}

// queue is the tasks pane: rows with a cursor and an order, and the geometry they
// are drawn through.
type queue struct {
	theme kit.Theme
	rows  *headless.Table[task]
	view  kit.Table
	// width is what the last frame was drawn at, which is what turns a press into a
	// column: a press arrives between two frames and is about the one on screen.
	width headless.Snapshot[int]
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
	q.rows.Row = q.row

	q.view = kit.Table{
		Theme:  theme,
		Glyphs: glyphs,
		Columns: []kit.Column{
			{Title: "task", Flex: 2, Min: 8},
			{Title: "state", Width: 9},
			{Title: "progress", Flex: 3, Min: 12},
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
	q.width.Stage(v, width)
	bands := v.Subs(layout.Down.Rects(v.Bounds().Size(),
		layout.Slot{Size: layout.Fixed(1)},
		layout.Slot{Size: layout.Flex(1)},
	))
	q.view.Titles(bands[0].View)
	q.rows.Draw(bands[1])
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
			column, on := q.view.ColumnAt(mouse.Pos.X, q.width.Value())
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
func (q *queue) row(v grid.View, at int, _ task, selected bool) {
	width, _ := v.Size()
	base := q.theme.Text
	if selected && q.rows.Focused() {
		base = base.Merge(q.theme.Selection)
		v.Fill(grid.Rect(0, 0, width, 1), q.theme.Selection)
	}
	q.view.Cells(v, at, base)
}

// cell draws one cell. The row index is the table's, so what is in a cell comes from
// this program's own rows and not from anything a widget copied.
func (q *queue) cell(v grid.View, row, column int, base grid.Style) {
	item, ok := q.rows.At(row)
	if !ok {
		return
	}
	switch column {
	case 0:
		kit.Label{Text: item.name, Style: base, Ellipsis: "…"}.Draw(v)
	case 1:
		kit.Label{Text: item.state, Style: base.Merge(q.state(item)), Ellipsis: "…"}.Draw(v)
	default:
		kit.Progress{
			Theme:   q.theme,
			Glyphs:  q.view.Glyphs,
			Done:    item.done,
			Total:   item.total,
			Percent: true,
		}.Draw(v)
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
	rate    *kit.Slider
}

func newActivity(theme kit.Theme, glyphs kit.Glyphs, of *queue) *activity {
	rate := kit.NewSlider(theme, glyphs, "rate", 1, 4)
	rate.Format = func(value int) string { return fmt.Sprintf("%d tasks/tick", value) }
	return &activity{theme: theme, glyphs: glyphs, of: of, rate: rate}
}

func (a *activity) tick() { a.spinner.Tick() }

func (a *activity) Measure(int) int { return 4 }

func (a *activity) Draw(v headless.Frame) {
	finished, total := a.of.remaining()
	rows := v.Subs(layout.Down.Rects(v.Bounds().Size(),
		layout.Slot{Size: layout.Fixed(1)},
		layout.Slot{Size: layout.Fixed(1)},
		layout.Slot{Size: layout.Fixed(1)},
		layout.Slot{Size: layout.Flex(1)},
	))
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
	a.spinner.Theme = a.theme
	a.spinner.Label = "watching"
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
