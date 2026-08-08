package kit

import (
	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/keymap"
	"github.com/Tangerg/oolong/core/layout"
)

// Settings dresses a browsable list of application-owned values.
//
// [headless.Settings] owns selection, scrolling and action routing. Label and Value
// are projections supplied by the application, so this component can display any
// setting without naming a product schema or storing a second copy of it. The value
// column is fitted to its widest cell and the label receives the remaining width.
type Settings[T any] struct {
	Theme Theme
	// Label and Value are the two columns shown for an item. Nil returns an empty
	// column.
	Label func(T) string
	Value func(T) string
	// ValueWidth caps the fitted value column. Zero leaves it uncapped.
	ValueWidth int

	controller *headless.Settings[T]
}

// NewSettings builds a settings list around one application-owned item slice.
func NewSettings[T any](
	theme Theme,
	items []T,
	label func(T) string,
	value func(T) string,
	change func(index int, item T, action keymap.Action) bool,
) *Settings[T] {
	controller := &headless.Settings[T]{Change: change}
	controller.SetItems(items)
	return &Settings[T]{Theme: theme, controller: controller, Label: label, Value: value}
}

// Controller returns the headless settings list that owns selection and actions.
func (s *Settings[T]) Controller() *headless.Settings[T] {
	if s == nil {
		return nil
	}
	return s.controller
}

// SetItems replaces the rows while preserving the selected index where possible.
func (s *Settings[T]) SetItems(items []T) {
	if s != nil && s.controller != nil {
		s.controller.SetItems(items)
	}
}

// Items returns a copy of the rows.
func (s *Settings[T]) Items() []T {
	if s == nil || s.controller == nil {
		return nil
	}
	return s.controller.Items()
}

// Current is the selected application item.
func (s *Settings[T]) Current() (T, bool) {
	if s != nil && s.controller != nil {
		return s.controller.Current()
	}
	var zero T
	return zero, false
}

// Selected is the selected row, or -1 when the list is empty.
func (s *Settings[T]) Selected() int {
	if s == nil || s.controller == nil {
		return -1
	}
	return s.controller.Selected()
}

// Scroll exposes the list's scrolling state.
func (s *Settings[T]) Scroll() *headless.Scroll {
	if s == nil || s.controller == nil {
		return nil
	}
	return s.controller.Scroll()
}

// Measure is one row per setting.
func (s *Settings[T]) Measure(int) int {
	if s == nil || s.controller == nil {
		return 0
	}
	return s.controller.Len()
}

// Draw paints the visible rows with one shared table layout.
func (s *Settings[T]) Draw(frame headless.Frame) {
	if s == nil || s.controller == nil {
		return
	}
	width, _ := frame.Size()
	table := s.table()
	columns := table.Layout(width)
	s.controller.DrawRows(frame, func(view grid.View, row int, _ T, selected bool) {
		base := s.Theme.Text
		if selected && s.controller.Focused() {
			base = base.Merge(s.Theme.Selection)
			view.Fill(view.Bounds(), s.Theme.Selection)
		}
		columns.Cells(view, row, base)
	})
}

// Handle routes navigation, pointer selection and value actions to the controller.
func (s *Settings[T]) Handle(event input.Event) bool {
	return s != nil && s.controller != nil && s.controller.Handle(event)
}

// Do runs a navigation or value action.
func (s *Settings[T]) Do(action keymap.Action) bool {
	return s != nil && s.controller != nil && s.controller.Do(action)
}

// Focus takes or releases the keyboard.
func (s *Settings[T]) Focus(has bool) {
	if s != nil && s.controller != nil {
		s.controller.Focus(has)
	}
}

func (s *Settings[T]) table() Table {
	return Table{
		Theme: s.Theme,
		Columns: []Column{
			{Size: layout.Flex(1).AtLeast(1)},
			{Align: layout.End, Size: layout.Measured(0, s.ValueWidth)},
		},
		Rows: s.controller.Len(),
		Cell: func(row, column int) Cell {
			item, ok := s.controller.At(row)
			if !ok {
				return Cell{}
			}
			if column == 0 {
				return LabelCell(Label{Text: project(s.Label, item), Ellipsis: "…"})
			}
			return LabelCell(Label{
				Text: project(s.Value, item), Style: s.Theme.Accent,
				Align: layout.End, Ellipsis: "…",
			})
		},
	}
}

func project[T any](f func(T) string, item T) string {
	if f == nil {
		return ""
	}
	return f(item)
}
