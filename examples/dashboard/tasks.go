package main

import (
	"cmp"
	"fmt"
	"time"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/kit"
)

// taskState orders the queue's state column: work not started, then running,
// then finished.
type taskState uint8

const (
	taskWaiting taskState = iota
	taskRunning
	taskDone
)

func (s taskState) String() string {
	switch s {
	case taskRunning:
		return "running"
	case taskDone:
		return "done"
	default:
		return "waiting"
	}
}

const (
	dashboardTickInterval = 120 * time.Millisecond
	activityHistoryLength = 64
	maximumUnitsPerTick   = 4
)

type task struct {
	name  string
	done  int
	total int
}

func (t task) state() taskState {
	switch {
	case t.done >= t.total:
		return taskDone
	case t.done > 0:
		return taskRunning
	default:
		return taskWaiting
	}
}

func formatRate(tasksPerTick int) string { return fmt.Sprintf("%d tasks/tick", tasksPerTick) }

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
				return kit.LabelCell(kit.Label{Text: item.state().String(), Style: q.stateStyle(item), Ellipsis: q.view.Glyphs.Ellipsis})
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
	style := q.theme.Text
	if selected && q.rows.Focused() {
		style = style.Merge(q.theme.Selection)
		v.Fill(grid.Area(0, 0, width, 1), q.theme.Selection)
	}
	columns.Cells(v, at, style)
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
	case taskDone:
		return q.theme.Success
	default:
		return q.theme.Muted
	}
}

func (q *queue) advance(by int) bool {
	if by <= 0 {
		return false
	}
	items := q.rows.Items()
	changed := false
	for i := range items {
		item := &items[i]
		if item.done >= item.total {
			continue
		}
		item.done = min(item.done+by, item.total)
		changed = true
	}
	if changed {
		q.rows.SetItems(items)
	}
	return changed
}

func (q *queue) completionTotals() (completedUnits, totalUnits int) {
	for _, item := range q.rows.Items() {
		completedUnits += item.done
		totalUnits += item.total
	}
	return completedUnits, totalUnits
}

func tasks() []task {
	return []task{
		{name: "build", done: 3, total: 12},
		{name: "test", done: 1, total: 20},
		{name: "vet", done: 0, total: 6},
		{name: "lint", done: 0, total: 9},
	}
}
