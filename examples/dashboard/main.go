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
		Root:     func(loop program.Loop) program.Component { return newDashboard(loop) },
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
	loop  program.Loop
	theme kit.Theme

	tabs  *headless.Tabs
	strip kit.Tabs
	work  *queue
	watch *activity
}

func newDashboard(loop program.Loop) *dashboard {
	theme := kit.Suited(loop.Ground())
	glyphs := kit.GlyphsFor(os.Getenv)

	d := &dashboard{loop: loop, theme: theme}
	d.work = newQueue(theme, glyphs)
	d.watch = &activity{theme: theme, glyphs: glyphs, of: d.work}

	d.tabs = &headless.Tabs{Items: []headless.Tab{
		{Title: "tasks", Of: d.work},
		{Title: "activity", Of: d.watch},
	}}
	d.strip = kit.Tabs{Of: d.tabs, Theme: theme, Glyphs: glyphs, Rule: true}
	d.tabs.Focus(true)

	// Something to watch: the work advances on a clock this program started, and an
	// interface with nothing animating asks for no frames at all.
	loop.Every(120*time.Millisecond, d.advance)
	return d
}

// Draw is the strip, whichever pane it says, and a hint row under both.
func (d *dashboard) Draw(v grid.View) {
	rows := layout.Rows(v,
		layout.Slot{Size: layout.Flex(1)},
		layout.Slot{Size: layout.Fixed(1)},
	)
	d.strip.Draw(rows[0])
	kit.Label{
		Text:  "alt+←/→: pane   ↑/↓: row   click a heading to sort   q: quit",
		Style: d.theme.Subtle,
	}.Draw(rows[1])
}

func (d *dashboard) Handle(ev input.Event) bool {
	if key, ok := ev.(input.Key); ok && key.Down() && key.Rune == 'q' {
		d.loop.Quit()
		return true
	}
	return d.strip.Handle(ev)
}

func (d *dashboard) advance() {
	d.work.advance()
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
	width int
}

func newQueue(theme kit.Theme, glyphs kit.Glyphs) *queue {
	q := &queue{theme: theme}
	q.rows = &headless.Table[task]{
		// One comparison for every column, which is what a table sorted by a column
		// somebody named needs.
		Less: func(a, b task, column int) bool {
			switch column {
			case 1:
				return a.state < b.state
			case 2:
				return a.done*b.total < b.done*a.total
			default:
				return a.name < b.name
			}
		},
	}
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
func (q *queue) Draw(v grid.View) {
	q.width, _ = v.Size()
	bands := layout.Rows(v,
		layout.Slot{Size: layout.Fixed(1)},
		layout.Slot{Size: layout.Flex(1)},
	)
	q.view.Titles(bands[0])
	q.rows.Draw(bands[1])
}

func (q *queue) Measure(int) int { return len(q.rows.Items) + 1 }

// Handle sorts by a heading that was pressed, and hands everything else to the rows
// — one row down, because the header took the first one.
func (q *queue) Handle(ev input.Event) bool {
	if mouse, ok := ev.(input.Mouse); ok {
		if mouse.Pos.Y == 0 {
			if mouse.Action != input.MouseDown {
				return false
			}
			column, on := q.view.ColumnAt(mouse.Pos.X, q.width)
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
	if row < 0 || row >= len(q.rows.Items) {
		return
	}
	item := q.rows.Items[row]
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
func (q *queue) advance() {
	for i := range q.rows.Items {
		item := &q.rows.Items[i]
		if item.done >= item.total {
			continue
		}
		item.done++
		item.state = running
		if item.done == item.total {
			item.state = done
		}
	}
}

// remaining is how much of the whole queue is left, which is what the other pane is
// about.
func (q *queue) remaining() (finished, total int) {
	for _, item := range q.rows.Items {
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
}

func (a *activity) tick() { a.spinner.Tick() }

func (a *activity) Measure(int) int { return 3 }

func (a *activity) Draw(v grid.View) {
	finished, total := a.of.remaining()
	rows := layout.Rows(v,
		layout.Slot{Size: layout.Fixed(1)},
		layout.Slot{Size: layout.Fixed(1)},
		layout.Slot{Size: layout.Flex(1)},
	)
	kit.Progress{
		Theme:   a.theme,
		Glyphs:  a.glyphs,
		Label:   "all of it",
		Done:    finished,
		Total:   total,
		Percent: true,
	}.Draw(rows[0])

	// A spinner is for work with no total and a bar is for work with one. Both are
	// here because the two questions are different: how far along, and is anything
	// happening at all.
	if finished >= total {
		kit.Label{Text: "everything is done", Style: a.theme.Success}.Draw(rows[2])
		return
	}
	a.spinner.Theme = a.theme
	a.spinner.Label = "watching"
	a.spinner.Draw(rows[2])
}

func tasks() []task {
	return []task{
		{name: "build", state: waiting, done: 3, total: 12},
		{name: "test", state: waiting, done: 1, total: 20},
		{name: "vet", state: waiting, done: 0, total: 6},
		{name: "lint", state: waiting, done: 0, total: 9},
	}
}
